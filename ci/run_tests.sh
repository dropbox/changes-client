#!/bin/bash -e
export GOPATH=~/
export PATH=$GOPATH/bin:/usr/local/go/bin:$PATH
WORKSPACE=$GOPATH/src/github.com/dropbox/changes-client
cd $WORKSPACE
# Just print vet issues for now.
vet -all . || true
# report non-'err' shadows
(vet -shadow -shadowstrict  . 2>&1 | grep -v "declaration of err") || true
go get github.com/jstemmer/go-junit-report
sudo CHANGES=1 PATH=$PATH GOPATH=$GOPATH `which go` test ./... -timeout=120s -v -race | tee test.log
echo Generating junit.xml...
go-junit-report -set-exit-code < test.log > junit.xml
