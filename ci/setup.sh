#!/bin/bash -e

support/bootstrap-ubuntu.sh
export PATH=/usr/local/go/bin:$PATH
export GOPATH=~/
cd $GOPATH/src/github.com/dropbox/changes-client
PATH=$PATH GOPATH=$GOPATH make dev
