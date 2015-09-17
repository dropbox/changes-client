// +build linux lxc

package lxcadapter

import (
	"log"
	"os"
	"sync"
	"testing"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/hashicorp/go-version"
	"gopkg.in/lxc/go-lxc.v2"

	. "gopkg.in/check.v1"
)

var (
	containerName string
)

func TestAdapter(t *testing.T) { TestingT(t) }

type AdapterSuite struct{}

var _ = Suite(&AdapterSuite{})

// we want to output the log from running the container
func (s *AdapterSuite) reportLogChunks(clientLog *client.Log) {
	for chunk, ok := clientLog.GetChunk(); ok; chunk, ok = clientLog.GetChunk() {
		log.Print(string(chunk))
	}
}

func (s *AdapterSuite) ensureContainerRemoved(c *C) {
	container, err := lxc.NewContainer(containerName, lxc.DefaultConfigPath())
	c.Assert(err, IsNil)
	defer lxc.Release(container)

	if container.Running() {
		log.Println("Existing test container running. Executing Stop()")
		err = container.Stop()
		c.Assert(err, IsNil)
	}
	c.Assert(container.Running(), Equals, false)

	if container.Defined() {
		log.Println("Existing test container present. Executing Destroy()")
		err = container.Destroy()
		c.Assert(err, IsNil)
	}
	c.Assert(container.Defined(), Equals, false)
}

func (s *AdapterSuite) SetUpSuite(c *C) {
	s.ensureContainerRemoved(c)
}

// For compatibility with existing deployments, any build of changes-client that uses
// the LXC adapter must use LXC at this version or above.
const minimumVersion = "1.1.2"

func (s *AdapterSuite) TestLxcVersion(c *C) {
	minVers, e := version.NewVersion(minimumVersion)
	if e != nil {
		panic(e)
	}
	currentVers, e := version.NewVersion(lxc.Version())
	if e != nil {
		c.Fatalf("Couldn't can't parse LXC version %q; %s", lxc.Version(), e)
	}
	if currentVers.LessThan(minVers) {
		c.Fatalf("Version must be >= %s; was %s", minimumVersion, lxc.Version())
	}
}

func (s *AdapterSuite) TestCompleteFlow(c *C) {
	var cmd *client.Command
	var err error
	var result *client.CommandResult

	if os.Getenv("CHANGES") == "1" {
		c.ExpectFailure("For as yet unknown reasons, container initialization fails on Changes.")
	}

	clientLog := client.NewLog()
	adapter, err := adapter.Create("lxc")
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

	// Set CpuLimit and MemoryLimit.
	// These values are usually set via flags that set `cpus` and `memory`.
	// This is to sanity check that the container doesn't fail to start with
	// reasonable values and our code for setting configs doesn't error out.
	// TODO: Should have tests that verify that these values have the desired effects.
	lxcAdapter, ok := adapter.(*Adapter)
	c.Assert(ok, Equals, true)
	lxcAdapter.container.CpuLimit = 1
	lxcAdapter.container.MemoryLimit = 512

	err = adapter.Prepare(clientLog)
	c.Assert(err, IsNil)
	defer adapter.Shutdown(clientLog)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\necho hello > foo.txt\nexit 0")
	c.Assert(err, IsNil)

	result, err = adapter.Run(cmd, clientLog)
	c.Assert(err, IsNil)
	c.Assert(string(result.Output), Equals, "")
	c.Assert(result.Success, Equals, true)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\necho $HOME\nexit 0")
	cmd.CaptureOutput = true
	c.Assert(err, IsNil)

	result, err = adapter.Run(cmd, clientLog)
	c.Assert(err, IsNil)
	c.Assert(string(result.Output), Equals, "/home/ubuntu\n")
	c.Assert(result.Success, Equals, true)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\ndd if=/dev/zero of=test.img bs=1M count=10 && mkfs.ext4 -b 1024 -j -F test.img && sudo mount -v -o loop test.img /mnt")
	cmd.CaptureOutput = true
	c.Assert(err, IsNil)

	result, err = adapter.Run(cmd, clientLog)
	c.Assert(err, IsNil)
	c.Assert(result.Success, Equals, true)

	// test with a command that expects stdin
	cmd, err = client.NewCommand("test", "#!/bin/bash -e\nread foo\nexit 1")
	c.Assert(err, IsNil)

	result, err = adapter.Run(cmd, clientLog)
	c.Assert(err, IsNil)
	c.Assert(string(result.Output), Equals, "")
	c.Assert(result.Success, Equals, false)

	artifacts, err := adapter.CollectArtifacts([]string{"foo.txt"}, clientLog)
	c.Assert(err, IsNil)
	c.Assert(len(artifacts), Equals, 1)
	c.Assert(artifacts[0], Matches, ".*/home/ubuntu/foo.txt")

	clientLog.Close()

	wg.Wait()
}

func init() {
	containerName = "84e6165919c04514a330fe789f367007"
}
