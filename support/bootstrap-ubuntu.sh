#!/bin/bash -eux

export DEBIAN_FRONTEND=noninteractive

GO_VERSION=1.3

sudo apt-get install -y python-software-properties software-properties-common
sudo add-apt-repository -y ppa:awstools-dev/awstools

sudo apt-get update -y

# Install basic requirements
sudo apt-get install -y git mercurial pkg-config wget

# Install aws cli tools
sudo apt-get install -y awscli

# Install go
set -ex
cd /tmp
wget "http://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz"
tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
echo 'export PATH=/usr/local/go/bin:$PATH' > /etc/profile.d/golang.sh
echo 'export GOPATH=~/' > /etc/profile.d/gopath.sh

# Install lxc
sudo apt-get install -y libcgmanager0 lxc lxc-dev

# Install fpm
sudo apt-get install -y ruby-dev gcc
sudo gem install fpm --no-ri --no-rdoc
