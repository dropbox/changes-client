// +build linux lxc
// +build !nolxc

package lxcadapter

import (
	"bytes"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"gopkg.in/lxc/go-lxc.v2"
	"io"
	"log"
	"os"
	"path/filepath"
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

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
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

	clientLog.Printf("Command exited with status %d", exitCode)

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
		clientLog.Printf("Failed to close all file descriptors! Ignoring and moving on..")
		break
	case <-sem:
		break
	}

	wg.Wait()

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
