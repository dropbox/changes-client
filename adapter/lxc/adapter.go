// +build linux lxc

package lxcadapter

import (
	"flag"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

var (
	preLaunch  string
	postLaunch string
	s3Bucket   string
)

type Adapter struct {
	config    *client.Config
	container *Container
}

func formatUUID(uuid string) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s", uuid[0:8], uuid[8:12], uuid[12:16], uuid[16:20], uuid[20:])
}

func (a *Adapter) Init(config *client.Config) error {
	container := &Container{
		Name:       formatUUID(config.JobstepID),
		Arch:       "amd64",
		Dist:       "ubuntu",
		Release:    "precise",
		PreLaunch:  preLaunch,
		PostLaunch: postLaunch,
		Snapshot:   formatUUID(config.Snapshot.ID),
		S3Bucket:   s3Bucket,
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
	return a.container.RunCommandInContainer(cmd, clientLog, "ubuntu")
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(clientLog *client.Log) error {
	return a.container.Destroy()
}

func init() {
	flag.StringVar(&preLaunch, "pre-launch", "", "Container pre-launch script")
	flag.StringVar(&postLaunch, "post-launch", "", "Container post-launch script")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "S3 bucket name")

	adapter.Register("lxc", &Adapter{})
}
