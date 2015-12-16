#!/bin/bash -eux

cd /vagrant/

support/bootstrap-ubuntu.sh

echo "alias work='cd \$GOPATH/src/github.com/dropbox/changes-client'" | sudo tee /etc/profile.d/work-alias.sh
sudo chown -R `whoami` ~/src
