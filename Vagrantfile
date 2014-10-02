# -*- mode: ruby -*-
# vi: set ft=ruby :

# Vagrantfile API/syntax version. Don't touch unless you know what you're doing!
VAGRANTFILE_API_VERSION = "2"

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.box = "ubuntu/trusty64"

  config.vm.provider "virtualbox" do |v|
    v.memory = 2048
    v.cpus = 4
  end

  config.ssh.forward_agent = true

  config.vm.synced_folder "./", "/home/vagrant/src/github.com/dropbox/changes-client"

  config.vm.provision :shell, :path => "support/bootstrap-vagrant.sh"
end
