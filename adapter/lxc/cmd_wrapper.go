// +build linux lxc

package lxcadapter

import (
	"bytes"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"gopkg.in/lxc/go-lxc.v1"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

type LxcCommand struct {
	command []string
	user    string
}

func NewLxcCommand(command []string, user string) *LxcCommand {
	return &LxcCommand{
		command: command,
		user:    user,
	}
}

func (cw *LxcCommand) Run(captureOutput bool, clientLog *client.Log, lxc *lxc.Container) (*client.CommandResult, error) {
	var err error

	// TODO(dcramer):
	clientLog.Writeln(fmt.Sprintf(">> %s", strings.Join(cw.command, " ")))

	inreader, inwriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	cmdreader, cmdwriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	var buffer *bytes.Buffer
	var reader io.Reader = cmdreader

	// If user has requested to buffer command output, tee output to in memory buffer.
	if captureOutput {
		buffer = &bytes.Buffer{}
		reader = io.TeeReader(cmdreader, buffer)
	}

	cmdwriterFd := cmdwriter.Fd()

	inreader.Close()
	inwriter.Close()

	cmdAsUser := generateCommand(cw.command, cw.user)

	log.Printf("[lxc] Executing %s", cmdAsUser)

	// TODO(dcramer): we are currently unable to get the exit status of
	// the command. https://github.com/lxc/go-lxc/issues/9
	err = lxc.RunCommandWithClearEnvironment(inwriter.Fd(), cmdwriterFd, cmdwriterFd, cmdAsUser...)
	if err != nil {
		clientLog.Writeln(fmt.Sprintf("Command failed: %s", err.Error()))
		cmdwriter.Close()
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		clientLog.WriteStream(reader)
	}()

	cmdwriter.Close()

	wg.Wait()

	result := &client.CommandResult{
		Success: true,
	}

	if captureOutput {
		result.Output = buffer.Bytes()
	} else {
		result.Output = []byte("")
	}
	return result, nil
}

func generateCommand(command []string, user string) []string {
	// TODO(dcramer):
	// homeDir := c.getHomeDir(user)
	// env = {
	//     # TODO(dcramer): HOME is pretty hacky here
	//     'USER': user,
	//     'HOME': home_dir,
	//     'PWD': cwd,
	//     'DEBIAN_FRONTEND': 'noninteractive',
	//     'LXC_NAME': self.name,
	//     'HOST_HOSTNAME': socket.gethostname(),
	//     'PATH': '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',
	// }
	//     if env:
	//         new_env.update(env)

	result := []string{"sudo", "-EHu", user}
	result = append(result, command...)
	return result
}
