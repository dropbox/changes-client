package basic

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/common"
	"os"
	"path/filepath"
)

type Adapter struct {
	config    *client.Config
	workspace string
}

func (a *Adapter) Init(config *client.Config) error {
	var err error
	var workspace string = config.Workspace

	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return err
	}

	a.config = config
	a.workspace = workspace

	return nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(clientLog *client.Log) error {
	return nil
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, clientLog *client.Log) (*client.CommandResult, error) {
	cw := client.NewCmdWrapper([]string{cmd.Path}, cmd.Cwd, cmd.Env)
	return cw.Run(cmd.CaptureOutput, clientLog)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(clientLog *client.Log) error {
	return nil
}

// If applicable, capture a snapshot of the workspace for later re-use
func (a *Adapter) CaptureSnapshot(outputSnapshot string, clientLog *client.Log) error {
	return nil
}

func (a *Adapter) CollectArtifacts(artifacts []string, clientLog *client.Log) ([]string, error) {
	return common.GlobTree(a.workspace, artifacts)
}

func init() {
	adapter.Register("basic", &Adapter{})
}
