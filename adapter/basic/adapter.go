package basic

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

type Adapter struct {
	config *client.Config
}

func (a *Adapter) Init(config *client.Config) error {
	a.config = config

	return nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(log *client.Log) error {
	return nil
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, log *client.Log) (*client.CommandResult, error) {
	cw := client.NewCmdWrapper([]string{cmd.Path}, cmd.Cwd, cmd.Env)
	return cw.Run(cmd.CaptureOutput, log)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(log *client.Log) error {
	return nil
}

func init() {
	adapter.Register("basic", &Adapter{})
}
