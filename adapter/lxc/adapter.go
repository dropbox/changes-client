package lxcadapter

import (
	"github.com/dropbox/changes-client/client"
)

type Adapter struct {
	config    *client.Config
	container *Container
}

func NewAdapter(config *client.Config) (*Adapter, error) {
	container, err := NewContainer(config.JobstepID)
	if err != nil {
		return nil, err
	}
	return &Adapter{
		config: config,
		// Reuse the UUID from the Jobstep as the container name
		container: container,
	}, nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(log *client.Log) error {
	return a.container.Launch(log)
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, log *client.Log) (*client.CommandResult, error) {
	return a.container.RunLocalScript(cmd.Path, cmd.CaptureOutput, log)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(log *client.Log) error {
	a.container.Destroy()
	return nil
}
