#!/bin/bash -e

sudo support/bootstrap-ubuntu.sh
sudo chown -R `whoami` ~
export PATH=/usr/local/go/bin:$PATH
export GOPATH=~/
WORKSPACE=$GOPATH/src/github.com/dropbox/changes-client
mkdir -p `dirname $WORKSPACE`
sudo cp -r . $WORKSPACE
cd $WORKSPACE
sudo PATH=$PATH GOPATH=$GOPATH make dev
sudo PATH=$PATH GOPATH=$GOPATH go get golang.org/x/tools/cmd/vet

# On Changes, don't bother running container puppet once setup has
# already run.
if [ ! -z $CHANGES ]; then
    touch /home/ubuntu/SKIP_CITOOLS_PUPPET
fi
