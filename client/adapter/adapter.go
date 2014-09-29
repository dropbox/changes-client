package adapter

import (
	"github.com/dropbox/changes-client/client"
)

type Adapter interface {
	Init(*client.Config) error
	Prepare(*client.Log) error
	Run(*client.Command, *client.Log) (*client.CommandResult, error)
	Shutdown(*client.Log) error
}
