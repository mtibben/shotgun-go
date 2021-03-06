package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	logger "log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// exit statuses
const (
	OK      = 0
	REBUILD = 1
	RESTART = 2
)

type WebProcess struct {
	CheckCmd  string
	BuildCmd  string
	RunCmd    string
	TargetUrl *url.URL
	command   *exec.Cmd
	output    bytes.Buffer
	stdout    io.Writer
	stderr    io.Writer
	m         sync.Mutex
	Log       *logger.Logger
}

func NewWebProcess(checkCmd, buildCmd, runCmd string, targeturl *url.URL, log *logger.Logger) *WebProcess {
	wp := &WebProcess{
		CheckCmd:  checkCmd,
		BuildCmd:  buildCmd,
		RunCmd:    runCmd,
		TargetUrl: targeturl,
		Log:       log,
	}
	wp.clearCmd()

	return wp
}

type responseWrapper struct {
	http.ResponseWriter
	*WebProcess
}

func (r responseWrapper) WriteHeader(code int) {
	r.ResponseWriter.WriteHeader(code)
	if code == http.StatusInternalServerError {
		io.Copy(r.ResponseWriter, &r.WebProcess.output)
	}
}

func (w *WebProcess) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := w.prepareProcessForRequest()
	if err != nil {
		// dump the output buffer to the http response
		output, _ := ioutil.ReadAll(&w.output)
		http.Error(rw, err.Error()+"\n\n"+string(output), http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(w.TargetUrl)
	proxy.ServeHTTP(responseWrapper{rw, w}, r)
}

func (w *WebProcess) prepareProcessForRequest() (err error) {
	w.m.Lock()
	defer w.m.Unlock()

	action := w.check()
	if !w.isRunning() || action == REBUILD {
		err = w.rebuildAndStart()
	} else if action == RESTART {
		err = w.restart()
	}

	return err
}

func (w *WebProcess) check() (exitstatus int) {
	result := exec.Command("bash", "-c", w.CheckCmd).Run()

	if result != nil {
		exiterr, ok := result.(*exec.ExitError)
		if ok {
			exitstatus = exiterr.Sys().(syscall.WaitStatus).ExitStatus()
			w.Log.Println("Check: got exit status", exitstatus)
		} else {
			panic("Couldn't case to ExitError")
		}
	}

	if exitstatus == REBUILD || exitstatus == RESTART {
		return exitstatus
	}

	return OK
}

func (w *WebProcess) restart() (err error) {
	w.Stop()
	err = w.start()
	if err != nil {
		w.Log.Println(err)
		return
	}
	err = w.waitUntilIsUp()
	return
}

func (w *WebProcess) rebuildAndStart() (err error) {
	err = w.rebuild()
	if err != nil {
		w.Log.Println(err)
		return
	}
	return w.restart()
}

func (w *WebProcess) Stop() {
	if w.command != nil {
		if w.command.Process != nil {
			w.Log.Println("Stopping pid", w.command.Process.Pid)
			w.command.Process.Kill()
		}
		w.clearCmd()
	}
}

func (w *WebProcess) clearCmd() {
	w.command = nil
	w.output = bytes.Buffer{}
	w.stdout = io.MultiWriter(&w.output, os.Stdout)
	w.stderr = io.MultiWriter(&w.output, os.Stderr)
}

func (w *WebProcess) rebuild() error {
	w.Log.Println("Build: " + w.BuildCmd)
	buildCmd := exec.Command("bash", "-c", w.BuildCmd)
	buildCmd.Stdout = w.stdout
	buildCmd.Stderr = w.stderr

	return buildCmd.Run()
}

func (w *WebProcess) start() error {
	w.Log.Println("Start: " + w.RunCmd)
	if w.isRunning() {
		return errors.New("Can't start, already running.")
	}

	w.command = exec.Command("bash", "-c", w.RunCmd)
	w.command.Stdout = w.stdout
	w.command.Stderr = w.stderr

	startErr := w.command.Start()
	if startErr != nil {
		return startErr
	}
	go w.command.Wait()

	w.Log.Println("Started pid", w.command.Process.Pid)
	return nil
}

func (w *WebProcess) isRunning() bool {
	return (w.command != nil) &&
		(w.command.Process != nil) &&
		((w.command.ProcessState == nil) || !w.command.ProcessState.Exited())
}

func (w *WebProcess) isUp() bool {
	_, err := http.Head(w.TargetUrl.String())
	return err == nil
}

func (w *WebProcess) waitUntilIsUp() error {
	if !w.isRunning() {
		return errors.New("Process is not running.")
	}

	w.Log.Println("Waiting for process...")
	ticker := time.NewTicker(time.Millisecond * 200)
	defer ticker.Stop()

	ticks := 0
	for _ = range ticker.C {
		if !w.isRunning() {
			return errors.New("Process not running")
		}
		if w.isUp() {
			return nil
		}
		w.Log.Print(".")
		ticks++
		if ticks > 20 {
			w.Log.Print("Giving up")
			return errors.New("Process did not listen after waiting 20*200ms")
		}
	}
	return nil
}
