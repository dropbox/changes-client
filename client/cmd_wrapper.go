package client

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type CmdWrapper struct {
	cmd *exec.Cmd
}

func NewCmdWrapper(command []string, cwd string, env []string) *CmdWrapper {
	c := exec.Command(command[0], command[1:]...)
	c.Env = env
	c.Dir = cwd
	return &CmdWrapper{
		cmd: c,
	}
}

func (cw *CmdWrapper) StdinPipe() (io.WriteCloser, error) {
	return cw.cmd.StdinPipe()
}

func (cw *CmdWrapper) CombinedOutputPipe() (io.ReadCloser, io.WriteCloser) {
	pr, pw := io.Pipe()

	cw.cmd.Stdout = pw
	cw.cmd.Stderr = pw

	return pr, pw
}

func (cw *CmdWrapper) Run(captureOutput bool, log *Log) (*CommandResult, error) {
	var err error

	stdin, err := cw.StdinPipe()
	if err != nil {
		return nil, err
	}

	cmdreader, cmdwriter := cw.CombinedOutputPipe()

	// TODO(dcramer):
	log.Writeln(fmt.Sprintf(">> %s", cw.cmd.Path))

	var buffer *bytes.Buffer
	var reader io.Reader = cmdreader

	// If user has requested to buffer command output, tee output to in memory buffer.
	if captureOutput {
		buffer = &bytes.Buffer{}
		reader = io.TeeReader(cmdreader, buffer)
	}

	err = cw.cmd.Start()

	stdin.Close()

	if err != nil {
		log.Writeln(fmt.Sprintf("Failed to start %s %s", cw.cmd.Args, err.Error()))
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		log.WriteStream(reader)
		wg.Done()
	}()

	err = cw.cmd.Wait()
	cmdwriter.Close()

	wg.Wait()

	if err != nil {
		// TODO(dcramer): what should we do here?
		log.Writeln(err.Error())
		return nil, err
	}

	result := &CommandResult{
		ProcessState: cw.cmd.ProcessState,
	}

	if captureOutput {
		result.Output = buffer.Bytes()
	} else {
		result.Output = []byte("")
	}
	return result, nil
}
