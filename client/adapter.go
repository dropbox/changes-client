package client

import (
	"os"
)

type Adapter interface {
	Prepare(*Log) error
	Run(*Command, *Log) (*os.ProcessState, error)
	Shutdown(*Log) error
}
