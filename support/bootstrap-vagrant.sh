#!/bin/bash -eux

export DEBIAN_FRONTEND=noninteractive

GO_VERSION=1.3

sudo apt-get update -y

# Install go
sudo apt-get install -y wget
set -ex
cd /tmp
wget "http://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz"
tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
echo 'export PATH=/usr/local/go/bin:$PATH' > /etc/profile.d/golang.sh
echo 'export GOPATH=~/' > /etc/profile.d/gopath.sh

# Install fpm
sudo apt-get install -y rubygems
sudo gem install fpm --no-ri --no-rdoc
