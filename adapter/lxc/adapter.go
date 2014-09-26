package lxc

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
)

type Adapter struct {
	config    *client.Config
	container *Container
}

func NewAdapter(config *client.Config) (*Adapter, error) {
	container, err := NewContainer(config.JobstepID)
	if err != nil {
		return err
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
	err := a.container.Create("ubuntu", "-a", "amd64", "-r", "precise")
	if err != nil {
		return err
	}
	return nil
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, log *client.Log) (*client.CommandResult, error) {
	dstFile := "/tmp/script"
	err := a.container.UploadFile(cmd.Path, dstFile)
	if err != nil {
		log.Writeln(fmt.Sprintf("Failed uploading script to container: %s", err.Error()))
		return nil, err
	}

	cw := NewLxcCommand([]string{"chmod", "0755", dstFile}, "ubuntu")
	_, err = cw.Run(false, log, a.lxc)
	if err != nil {
		return nil, err
	}

	cw = NewLxcCommand([]string{dstFile}, "ubuntu")
	return cw.Run(cmd.CaptureOutput, log, a.lxc)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(log *client.Log) error {
	return nil
}
