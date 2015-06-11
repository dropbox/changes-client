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

The binary will be installed at `./bin/client` folder


Example Run
-----------


```
./bin/client --server "https://changes.build.itc.dropbox.com/api/0" --jobstep_id "bbc9a199-1b36-4f7d-9072-3974f32fdb1b"
```

> NOTE: There is no `/` at the end of `--server`


Development
-----------

A Vagrant VM is included to make development easy:

```
$ vagrant up --provision
```

Jump into the VM with `vagrant ssh`, and then use the `work` alias to hop into the environment:

```
$ work
$ make dev
$ make test
```


Building package
----------------

We use [fpm](https://github.com/jordansissel/fpm) to build our deb file.

```
$ work
$ make deb
```

Thats it. A `.deb` file should be available as changes-client\_$VERSION\_amd64.deb

Note that the LXC you build against needs to match prod.
