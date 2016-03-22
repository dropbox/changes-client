// +build linux lxc

package lxcadapter

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dropbox/changes-client/client"
	"gopkg.in/lxc/go-lxc.v2"
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

func timedWait(fn func(), timeout time.Duration) error {
	complete := make(chan bool)

	go func() {
		fn()
		complete <- true
	}()

	select {
	case <-time.After(timeout):
		return fmt.Errorf("Timed out waiting running method: %v", fn)
	case <-complete:
		return nil
	}
}

func (cw *LxcCommand) Run(captureOutput bool, clientLog *client.Log, container *lxc.Container) (*client.CommandResult, error) {
	clientLog.Printf("==> Executing %s", strings.Join(cw.Args, " "))

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

	cwd := cw.Cwd
	// ensure cwd is an absolute path
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(homeDir, cwd)
	}

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

	var clientLogClosed sync.WaitGroup
	clientLogClosed.Add(1)
	go func() {
		defer clientLogClosed.Done()
		clientLog.WriteStream(reader)
	}()

	log.Printf("[lxc] Executing %s from [%s]", cmdAsUser, cwd)
	exitCode, err := container.RunCommandStatus(cmdAsUser, lxc.AttachOptions{
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
		clientLog.Printf("Running the command failed: %s", err)
		cmdwriter.Close()
		return nil, err
	}

	// Wait 10 seconds for the pipe to close. If it doesn't we give up on actually closing
	// as a child process might be causing things to stick around.
	// XXX: this logic is duplicated in client.CmdWrapper
	if timedWait(func() {
		if err := cmdwriter.Close(); err != nil {
			clientLog.Printf("Error closing writer FD: %s", err)
		}
	}, 10*time.Second) != nil {
		clientLog.Printf("Failed to close all file descriptors! Ignoring and moving on..")
	}

	// If the container dup'd the file descriptors, closing cmdwriter doesn't close the stream.
	// cmdreader (at the other end of the OS pipe) will only close when all duplicates of cmdwriter
	// are closed.
	//
	// We've seen this hang happen just after phantomjs execution in the container - could just be a
	// coincidence.
	//
	// To avoid hanging forever waiting for the reader to close, add a timeout to the waitgroup Wait().
	if timedWait(clientLogClosed.Wait, 5*time.Second) != nil {
		clientLog.Printf("Timed out waiting for waitGroup to complete")
	}

	clientLog.Printf("Command exited with status %d", exitCode)

	result := &client.CommandResult{
		Success: exitCode == 0,
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
		return filepath.Join("/home", user)
	}
}
