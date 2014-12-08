package runner

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
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

func (cw *CmdWrapper) Run(captureOutput bool, clientLog *Log) (*CommandResult, error) {
	var err error

	stdin, err := cw.StdinPipe()
	if err != nil {
		return nil, err
	}

	cmdreader, cmdwriter := cw.CombinedOutputPipe()

	// TODO(dcramer):
	clientLog.Writeln(fmt.Sprintf("==> Executing %s", cw.cmd.Args))

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
		clientLog.Writeln(fmt.Sprintf("Failed to start %s %s", cw.cmd.Args, err.Error()))
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		clientLog.WriteStream(reader)
		wg.Done()
	}()

	err = cw.cmd.Wait()

	// Wait 10 seconds for the pipe to close. If it doesn't we give up on actually closing
	// as a child process might be causing things to stick around.
	// XXX: this logic is duplicated in lxcadapter.CmdWrapper
	timeLimit := time.After(10 * time.Second)
	sem := make(chan struct{}) // lol struct{} is cheaper than bool
	go func() {
		cmdwriter.Close()
		sem <- struct{}{}
	}()

	select {
	case <-timeLimit:
		clientLog.Writeln(fmt.Sprintf("Failed to close all file descriptors! Ignoring and moving on.."))
		break
	case <-sem:
		break
	}

	wg.Wait()

	if err != nil {
		// TODO(dcramer): what should we do here?
		clientLog.Writeln(err.Error())
		return nil, err
	}

	result := &CommandResult{
		Success: cw.cmd.ProcessState.Success(),
	}

	if captureOutput {
		result.Output = buffer.Bytes()
	} else {
		result.Output = []byte("")
	}
	return result, nil
}
