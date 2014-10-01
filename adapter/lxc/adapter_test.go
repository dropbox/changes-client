// +build linux lxc

package lxcadapter

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"gopkg.in/lxc/go-lxc.v2"
	"log"
	"sync"
	"testing"

    . "gopkg.in/check.v1"
)

var (
	containerName string
)

func Test(t *testing.T) { TestingT(t) }

type AdapterSuite struct{}

var _ = Suite(&AdapterSuite{})

// we want to output the log from running the container
func (s *AdapterSuite) reportLogChunks(clientLog *client.Log) {
	for chunk := range clientLog.Chan {
		log.Print(string(chunk))
	}
}

func (s *AdapterSuite) ensureContainerRemoved(c *C) {
	container, err := lxc.NewContainer(containerName, lxc.DefaultConfigPath())
	c.Assert(err, IsNil)
	defer lxc.PutContainer(container)

	if container.Running() {
		err = container.Stop()
		c.Assert(err, IsNil)
	}
	c.Assert(container.Running(), Equals, false)

	if container.Defined() {
		err = container.Destroy()
		c.Assert(err, IsNil)
	}
	c.Assert(container.Defined(), Equals, false)
}


func (s *AdapterSuite) SetUpSuite(c *C) {
	s.ensureContainerRemoved(c)
}

func (s *AdapterSuite) TestCompleteFlow(c *C) {
	var cmd *client.Command
	var err error
	var result *client.CommandResult

	clientLog := client.NewLog()
	adapter, err := adapter.Get("lxc")
	c.Assert(err, IsNil)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.reportLogChunks(clientLog)
	}()

	config := &client.Config{}
	config.JobstepID = containerName

	err = adapter.Init(config)
	c.Assert(err, IsNil)

	err = adapter.Prepare(clientLog)
	c.Assert(err, IsNil)
	defer adapter.Shutdown(clientLog)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\necho hello\nexit 0")
	cmd.CaptureOutput = true
	c.Assert(err, IsNil)

	result, err = adapter.Run(cmd, clientLog)
	c.Assert(err, IsNil)
	c.Assert(string(result.Output), Equals, "hello\n")
	c.Assert(result.Success, Equals, true)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\necho hello\nexit 1")
	c.Assert(err, IsNil)

	result, err = adapter.Run(cmd, clientLog)
	c.Assert(err, IsNil)
	c.Assert(string(result.Output), Equals, "")
	c.Assert(result.Success, Equals, false)

	clientLog.Close()

	wg.Wait()
}

func init() {
	containerName = "changes-client-test"
}
