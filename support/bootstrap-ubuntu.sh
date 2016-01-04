#!/bin/bash -ex

export DEBIAN_FRONTEND=noninteractive

GO_VERSION=1.3

sudo apt-get install -y python-software-properties software-properties-common
sudo add-apt-repository -y ppa:awstools-dev/awstools

# If we're running on Changes, don't pick up LXC ppa
if [ -z $CHANGES ]
then
  sudo add-apt-repository -y ppa:ubuntu-lxc/stable
fi

sudo apt-get update -y

# Install basic requirements
sudo apt-get install -y git mercurial pkg-config wget

# Install aws cli tools
sudo apt-get install -y awscli

# Install go
if [ ! -x /usr/local/go/bin/go ]
then
  echo "Installing Go binary...."
  cd /tmp
  wget "http://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
  echo "Installed Go binary...."
else
  echo "Go binary already installed"
fi

/usr/local/go/bin/go version

# Install lxc
sudo apt-get install -y libcgmanager0 lxc lxc-dev

# Install fpm
sudo apt-get install -y ruby-dev gcc
fpm -h > /dev/null || sudo gem install fpm --no-ri --no-rdoc
