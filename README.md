shotgun-go
==========

Shotgun-go is an automatic code reloading dev server for go. 

Shotgun-go listens for http requests but does not load the application. When a request is recieved, the application is  built and run. Each request triggers a check for changes, and if necessary a compile and reload of the application.

Using
------
Example Usage:
```
shotgun -u http://localhost:8008 -p 8010 \
    -checkCmd='exit `find -name *.go -newer ./bin/myapp | wc -l`' \
    -buildCmd="go build -o ./bin/myapp myapp" \
    -runCmd="./bin/myapp"`
```

Arguments:
- `-url` or `-u` The url to proxy for
- `-port` or `-p` Port for shotgun to listen for http requests on
- `-checkCmd` command to check for changes. Return 0 for no change, 1 for rebuild, and 2 for restart.
- `-buildCmd` command to build the binary.
- `-runCmd` command to run the binary.

Alternatively, use a yml config file named `.shotgun-go` and run `shotgun`
```yml
env:
  - FOO: "bar"
port: 8010
url: http://localhost:8008
checkcmd: "exit `find -name *.go -newer ./bin/myapp | wc -l`
buildcmd: "go build -o ./bin/myapp myapp"
runcmd: "./bin/myapp"
```
