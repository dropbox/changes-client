package lxc

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
)

type Adapter struct {
	config    *client.Config
	container *Container
}

func NewAdapter(config *client.Config) *Adapter {
	return &Adapter{
		config: config,
		// Reuse the UUID from the Jobstep as the container name
		container: NewContainer(config.JobstepID),
	}
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(log *client.Log) error {
	return nil
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, log *client.Log) (*client.CommandResult, error) {
	return a.runCommandInContainer(cmd, log)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(log *client.Log) error {
	return nil
}

// Runs a local script within the container.
func (a *Adapter) runCommandInContainer(cmd *client.Command, log *client.Log) (*client.CommandResult, error) {
	dstFile := "/tmp/script"
	err := a.container.UploadFile(cmd.Path, dstFile)
	if err != nil {
		log.Writeln(fmt.Sprintf("Failed uploading script to container: %s", err.Error()))
		return nil, err
	}

	lxcCmd := a.container.GenerateCommand([]string{"chmod", "0755", dstFile}, "ubuntu")
	cw := client.NewCmdWrapper(lxcCmd, "", []string{})
	_, err = cw.Run(cmd.CaptureOutput, log)
	if err != nil {
		return nil, err
	}

	// TODO(dcramer): ubuntu needs configurable
	lxcCmd = a.container.GenerateCommand([]string{dstFile}, "ubuntu")
	cw = client.NewCmdWrapper(lxcCmd, "", []string{})
	return cw.Run(cmd.CaptureOutput, log)
}
