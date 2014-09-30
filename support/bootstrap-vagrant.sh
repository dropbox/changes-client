#!/bin/bash -eux

cd /vagrant/

support/bootstrap-ubuntu.sh

echo "alias work='cd \$GOPATH/src/github.com/dropbox/changes-client'" > /etc/profile.d/work-alias.sh
