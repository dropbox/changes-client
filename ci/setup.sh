#!/bin/bash -e

sudo -E support/bootstrap-ubuntu.sh
sudo chown -R `whoami` ~
export PATH=/usr/local/go/bin:$PATH
export GOPATH=~/
WORKSPACE=$GOPATH/src/github.com/dropbox/changes-client
mkdir -p `dirname $WORKSPACE`
sudo cp -r . $WORKSPACE
cd $WORKSPACE
sudo PATH=$PATH GOPATH=$GOPATH make dev
sudo PATH=$PATH GOPATH=$GOPATH go get golang.org/x/tools/cmd/vet
