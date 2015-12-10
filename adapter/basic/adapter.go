package basic

import (
	"path/filepath"

	autil "github.com/dropbox/changes-client/adapter"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

type Adapter struct {
	config    *client.Config
	workspace string
}

func (a *Adapter) Init(config *client.Config) error {
	if workspace, err := filepath.Abs(config.ArtifactSearchPath); err != nil {
		return err
	} else {
		a.workspace = workspace
	}

	a.config = config
	return nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(clientLog *client.Log) (client.Metrics, error) {
	return nil, nil
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

func (a *Adapter) GetRootFs() string {
	return "/"
}

func (a *Adapter) CollectArtifacts(artifacts []string, clientLog *client.Log) ([]string, error) {
	return autil.CollectArtifactsIn(a.workspace, artifacts, clientLog)
}

func (a *Adapter) GetArtifactRoot() string {
	return a.workspace
}

func New() adapter.Adapter {
	return &Adapter{}
}

func init() {
	adapter.Register("basic", New)
}
