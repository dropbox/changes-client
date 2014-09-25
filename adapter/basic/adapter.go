package basic

import (
	"github.com/dropbox/changes-client/client"
	"os"
)

type Adapter struct {
	config *client.Config
}

func NewAdapter(config *client.Config) *Adapter {
	return &Adapter{
		config: config,
	}
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare() error {
	return nil
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command) (*os.ProcessState, error) {
	return cmd.Run()
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown() error {
	return nil
}
