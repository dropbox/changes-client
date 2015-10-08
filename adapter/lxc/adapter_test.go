// +build linux lxc
// +build !nolxc

package lxcadapter

import (
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/hashicorp/go-version"
	"gopkg.in/lxc/go-lxc.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// we want to output the log from running the container
func reportLogChunks(clientLog *client.Log) {
	for chunk, ok := clientLog.GetChunk(); ok; chunk, ok = clientLog.GetChunk() {
		log.Print(string(chunk))
	}
}

func ensureContainerRemoved(t *testing.T, name string) {
	container, err := lxc.NewContainer(name, lxc.DefaultConfigPath())
	require.NoError(t, err)
	defer lxc.Release(container)

	if container.Running() {
		log.Println("Existing test container running. Executing Stop()")
		require.NoError(t, container.Stop())
	}
	require.Equal(t, container.Running(), false)

	if container.Defined() {
		log.Println("Existing test container present. Executing Destroy()")
		require.NoError(t, container.Destroy())
	}
	require.False(t, container.Defined())
}

// For compatibility with existing deployments, any build of changes-client that uses
// the LXC adapter must use LXC at this version or above.
const minimumVersion = "1.1.2"

func TestLxcVersion(t *testing.T) {
	minVers, e := version.NewVersion(minimumVersion)
	if e != nil {
		panic(e)
	}
	currentVers, e := version.NewVersion(lxc.Version())
	require.Nil(t, e, "Couldn't can't parse LXC version %q; %s", lxc.Version(), e)
	require.False(t, currentVers.LessThan(minVers), "Version must be >= %s; was %s", minimumVersion, lxc.Version())
}

const containerName = "84e6165919c04514a330fe789f367007"

func TestCompleteFlow(t *testing.T) {
	if os.Getenv("CHANGES") == "1" {
		t.Skip("For as yet unknown reasons, container initialization fails on Changes.")
	}

	ensureContainerRemoved(t, containerName)

	clientLog := client.NewLog()
	adapter, err := adapter.Create("lxc")
	require.NoError(t, err)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		reportLogChunks(clientLog)
	}()

	config := &client.Config{
		JobstepID: containerName,
	}

	require.NoError(t, adapter.Init(config))

	// Set CpuLimit and MemoryLimit.
	// These values are usually set via flags that set `cpus` and `memory`.
	// This is to sanity check that the container doesn't fail to start with
	// reasonable values and our code for setting configs doesn't error out.
	// TODO: Should have tests that verify that these values have the desired effects.
	lxcAdapter, ok := adapter.(*Adapter)
	require.True(t, ok)
	lxcAdapter.container.CpuLimit = 1
	lxcAdapter.container.MemoryLimit = 512

	require.NoError(t, adapter.Prepare(clientLog))
	defer adapter.Shutdown(clientLog)

	cmd, err := client.NewCommand("test", "#!/bin/bash -e\necho hello > foo.txt\nexit 0")
	require.NoError(t, err)

	var result *client.CommandResult
	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	require.Equal(t, string(result.Output), "")
	require.True(t, result.Success)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\necho $HOME\nexit 0")
	cmd.CaptureOutput = true
	require.NoError(t, err)

	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	require.Equal(t, string(result.Output), "/home/ubuntu\n")
	require.True(t, result.Success)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\ndd if=/dev/zero of=test.img bs=1M count=10 && mkfs.ext4 -b 1024 -j -F test.img && sudo mount -v -o loop test.img /mnt")
	cmd.CaptureOutput = true
	require.NoError(t, err)

	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	require.True(t, result.Success)

	// test with a command that expects stdin
	cmd, err = client.NewCommand("test", "#!/bin/bash -e\nread foo\nexit 1")
	require.NoError(t, err)

	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	require.Equal(t, string(result.Output), "")
	require.True(t, result.Success)

	artifacts, err := adapter.CollectArtifacts([]string{"foo.txt"}, clientLog)
	require.NoError(t, err)
	require.Equal(t, len(artifacts), 1)
	require.Regexp(t, artifacts[0], ".*/home/ubuntu/foo.txt")

	clientLog.Close()

	wg.Wait()
}

func TestDebugKeep(t *testing.T) {
	clientLog := client.NewLog()
	go func() {
		clientLog.Drain()
	}()
	{
		future := time.Now().Add(10 * time.Minute)
		cfg1, e := client.LoadConfig([]byte(`{"debugConfig":{"lxc_keep_container_end_rfc3339": "` + future.Format(time.RFC3339) + `"}}`))
		if e != nil {
			panic(e)
		}
		assert.True(t, shouldDebugKeep(clientLog, cfg1))
	}

	{
		past := time.Now().Add(-10 * time.Minute)
		cfg2, e := client.LoadConfig([]byte(`{"debugConfig":{"lxc_keep_container_end_rfc3339": "` + past.Format(time.RFC3339) + `"}}`))
		if e != nil {
			panic(e)
		}
		assert.False(t, shouldDebugKeep(clientLog, cfg2))
	}

	assert.False(t, shouldDebugKeep(clientLog, new(client.Config)))
}
