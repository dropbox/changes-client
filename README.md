Changes Client
==============

Can be used to run arbitrary commands. You will need standard
Go code setup to compile this.

Setup
-----

```
mkdir -p go/src go/bin go/pkg
cd go
GOPATH=`pwd` go get github.com/dropbox/changes-client
```

Build
-----

```
go install github.com/dropbox/changes-client/client
```

The binary will be installed at ./bin/client folder

Example Run
-----------


```
./bin/client --server "https://changes.build.itc.dropbox.com/api/0" --jobstep_id "bbc9a199-1b36-4f7d-9072-3974f32fdb1b"
```

> NOTE: There is no `/` at the end of `--server`
