package adapter

import (
	"fmt"
	"github.com/dropbox/changes-client/shared/runner"
)

type Adapter interface {
	Init(*runner.Config) error
	Prepare(*runner.Log) error
	Run(*runner.Command, *runner.Log) (*runner.CommandResult, error)
	Shutdown(*runner.Log) error
	CaptureSnapshot(string, *runner.Log) error
	CollectArtifacts([]string, *runner.Log) ([]string, error)
}

func FormatUUID(uuid string) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s", uuid[0:8], uuid[8:12], uuid[12:16], uuid[16:20], uuid[20:])
}
