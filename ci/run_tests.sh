#!/bin/bash -e
export PATH=/usr/local/go/bin:$PATH
export GOPATH=~/
WORKSPACE=$GOPATH/src/github.com/dropbox/changes-client
cd $WORKSPACE
sudo PATH=$PATH GOPATH=$GOPATH make test
