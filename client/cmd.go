package client

import (
	"io/ioutil"
	"os"
)

type Command struct {
	ID            string
	Path          string
	Env           []string
	Cwd           string
	CaptureOutput bool
}

type CommandResult struct {
	Output       []byte // buffered output if requested
	ProcessState *os.ProcessState
}

func (cr *CommandResult) Success() bool {
	return cr.ProcessState.Success()
}

// Build a new Command out of an arbitrary script
// The script is written to disk and then executed ensuring that it can
// be fairly arbitrary and provide its own shebang
func NewCommand(id string, script string) (*Command, error) {
	f, err := ioutil.TempFile("", "script-")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = f.WriteString(script)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	err = f.Chmod((info.Mode() & os.ModePerm) | 0111)
	if err != nil {
		return nil, err
	}

	// TODO(dcramer): generate a better name
	return &Command{
		ID:   id,
		Path: f.Name(),
	}, nil
}
