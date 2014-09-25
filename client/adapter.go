package client

import (
	"os"
)

type Adapter interface {
	Prepare() error
	Run(*Command) (*os.ProcessState, error)
	Shutdown() error
}
