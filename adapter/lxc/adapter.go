package lxc

import (
	"github.com/dropbox/changes-client/client"
)

type Adapter struct {
	config *client.Config
	containerName string
}

func NewAdapter(config *client.Config) *Adapter {
	return &Adapter{
		config: config,
		// Reuse the UUID from the Jobstep as the container name
		containerName: config.JobstepID,
	}
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
