// +build linux lxc

package lxcadapter

import (
	"bytes"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"gopkg.in/lxc/go-lxc.v2"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type LxcCommand struct {
	Args []string
	User string
	Env  []string
	Cwd  string
}

func NewLxcCommand(args []string, user string) *LxcCommand {
	return &LxcCommand{
		Args: args,
		User: user,
	}
}

func (cw *LxcCommand) Run(captureOutput bool, clientLog *client.Log, container *lxc.Container) (*client.CommandResult, error) {
	var err error

	// TODO(dcramer):
	clientLog.Writeln(fmt.Sprintf("==> Executing %s", strings.Join(cw.Args, " ")))

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

	cmdAsUser := generateCommand(cw.Args, cw.User)

	homeDir := getHomeDir(cw.User)

	// we want to ensure that our path is always treated as relative to our
	// home directory
	cwd := path.Join(homeDir, cw.Cwd)

	env := []string{
		fmt.Sprintf("USER=%s", cw.User),
		// TODO(dcramer): HOME is pretty hacky here
		fmt.Sprintf("HOME=%s", homeDir),
		fmt.Sprintf("PWD=%s", cwd),
		fmt.Sprintf("DEBIAN_FRONTEND=noninteractive"),
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
	for i := 0; i < len(cw.Env); i++ {
		env = append(env, cw.Env[i])
	}

	// TODO(dcramer): we are currently unable to get the exit status of
	// the command. https://github.com/lxc/go-lxc/issues/9

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		clientLog.WriteStream(reader)
	}()

	log.Printf("[lxc] Executing %s from [%s]", cmdAsUser, cwd)
	ok, err := container.RunCommand(cmdAsUser, lxc.AttachOptions{
		StdinFd:    inwriter.Fd(),
		StdoutFd:   cmdwriterFd,
		StderrFd:   cmdwriterFd,
		Env:        env,
		Cwd:        cwd,
		Arch:       lxc.X86_64,
		Namespaces: -1,
		UID:        -1,
		GID:        -1,
		ClearEnv:   true,
	})
	if err != nil {
		clientLog.Writeln(fmt.Sprintf("Command failed: %s", err.Error()))
		cmdwriter.Close()
		return nil, err
	}

	// Wait 10 seconds for the pipe to close. If it doesn't we give up on actually closing
	// as a child process might be causing things to stick around.
	// XXX: this logic is duplicated in client.CmdWrapper
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

	result := &client.CommandResult{
		Success: ok,
	}

	if captureOutput {
		result.Output = buffer.Bytes()
	} else {
		result.Output = []byte("")
	}
	return result, nil
}

func generateCommand(args []string, user string) []string {
	if user == "root" {
		return args
	}

	result := []string{"sudo", "-EHu", user}
	result = append(result, args...)
	return result
}

func getHomeDir(user string) string {
	if user == "root" {
		return "/root"
	} else {
		return fmt.Sprintf("/home/%s", user)
	}
}
