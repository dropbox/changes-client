// +build linux lxc

package lxcadapter

import (
	"flag"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

var (
	preLaunch  string
	postLaunch string
)

type Adapter struct {
	config    *client.Config
	container *Container
}

func (a *Adapter) Init(config *client.Config) error {
	container, err := NewContainer(config.JobstepID, preLaunch, postLaunch)
	if err != nil {
		return err
	}

	a.config = config
	a.container = container

	return nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(clientLog *client.Log) error {
	return a.container.Launch(clientLog)
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, clientLog *client.Log) (*client.CommandResult, error) {
	return a.container.RunLocalScript(cmd.Path, cmd.CaptureOutput, clientLog)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(clientLog *client.Log) error {
	return a.container.Destroy()
}

func init() {
	flag.StringVar(&preLaunch, "pre-launch", "", "Container pre-launch script")
	flag.StringVar(&postLaunch, "post-launch", "", "Container post-launchs cript")

	adapter.Register("lxc", &Adapter{})
}
