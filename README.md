Changes Client
==============

Can be used to run arbitrary commands. You will need standard
Go code setup to compile this.

Setup
-----

```
mkdir -p go/src go/bin go/pkg
cd go
export GOPATH=`pwd`

go get github.com/dropbox/changes-client
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

Building package
----------------

We will use [fpm](https://github.com/jordansissel/fpm) to build our deb file.

```
mkdir -p /tmp/changes-client-build/usr/bin
cp ./bin/client /tmp/changes-client-build/usr/bin/changes-client
fpm -s dir -t deb -n "changes-client" -v $VERSION /tmp/changes-client
```

Thats it. `.deb` file should be available as changes-client\_$VERSION\_amd64.deb
