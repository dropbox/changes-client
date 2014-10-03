package adapter

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
)

type Adapter interface {
	Init(*client.Config) error
	Prepare(*client.Log) error
	Run(*client.Command, *client.Log) (*client.CommandResult, error)
	Shutdown(*client.Log) error
	CaptureSnapshot(string, *client.Log) error
}

func FormatUUID(uuid string) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s", uuid[0:8], uuid[8:12], uuid[12:16], uuid[16:20], uuid[20:])
}
