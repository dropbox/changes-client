#!/bin/bash -eu
export GOPATH=~/
export PATH=$GOPATH/bin:/usr/local/go/bin:$PATH
WORKSPACE=$GOPATH/src/github.com/dropbox/changes-client
cd $WORKSPACE

echo Running vet...
# TODO(anupc): Remove -printf=false
# Fails with 'adapter/lxc/cmd_wrapper.go:44: arg fn in printf call is a function value, not a function call'
go tool vet -all -printf=false .
echo Done.
# report non-'err' shadows
(go tool vet -shadow -shadowstrict  . 2>&1 | grep -v "declaration of err") || true

go get github.com/jstemmer/go-junit-report
sudo CHANGES=1 PATH=$PATH GOPATH=$GOPATH `which go` test -bench . ./... -timeout=120s -v -race | tee test.log
EXIT_CODE=${PIPESTATUS[0]}
echo Generating junit.xml...
go-junit-report < test.log > junit.xml
echo Done.
exit ${EXIT_CODE}
