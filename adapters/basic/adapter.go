package basicadapter

import (
	"github.com/dropbox/changes-client/shared/adapter"
	"github.com/dropbox/changes-client/shared/glob"
	"github.com/dropbox/changes-client/shared/runner"
	"os"
	"path/filepath"
)

type Adapter struct {
	config    *runner.Config
	workspace string
}

func (a *Adapter) Init(config *runner.Config) error {
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
func (a *Adapter) Prepare(clientLog *runner.Log) error {
	return nil
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *runner.Command, clientLog *runner.Log) (*runner.CommandResult, error) {
	cw := runner.NewCmdWrapper([]string{cmd.Path}, cmd.Cwd, cmd.Env)
	return cw.Run(cmd.CaptureOutput, clientLog)
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(clientLog *runner.Log) error {
	return nil
}

// If applicable, capture a snapshot of the workspace for later re-use
func (a *Adapter) CaptureSnapshot(outputSnapshot string, clientLog *runner.Log) error {
	return nil
}

func (a *Adapter) CollectArtifacts(artifacts []string, clientLog *runner.Log) ([]string, error) {
	return glob.GlobTree(a.workspace, artifacts)
}

func init() {
	adapter.Register("basic", &Adapter{})
}
