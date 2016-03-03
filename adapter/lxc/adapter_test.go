// +build linux lxc
// +build !nolxc

package lxcadapter

import (
	"log"
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

	_, err = adapter.Prepare(clientLog)
	require.NoError(t, err)

	cmd, err := client.NewCommand("test", "#!/bin/bash -e\necho hello > foo.txt\nexit 0")
	require.NoError(t, err)

	var result *client.CommandResult
	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	require.Equal(t, "", string(result.Output))
	require.True(t, result.Success)

	cmd, err = client.NewCommand("test", "#!/bin/bash -e\necho $HOME\nexit 0")
	cmd.CaptureOutput = true
	require.NoError(t, err)

	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	require.Equal(t, "/home/ubuntu\n", string(result.Output))
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
	require.Equal(t, "", string(result.Output))
	require.False(t, result.Success)

	artifacts, err := adapter.CollectArtifacts([]string{"foo.txt"}, clientLog)
	require.NoError(t, err)
	require.Equal(t, 1, len(artifacts))
	require.Regexp(t, ".*/home/ubuntu/foo.txt", artifacts[0])

	// test that blacklist-remove is successfully mounted in the container
	// and can be run inside it.
	cmd, err = client.NewCommand("test", "/var/changes/input/blacklist-remove nonexistent.yaml")
	require.NoError(t, err)

	result, err = adapter.Run(cmd, clientLog)
	require.NoError(t, err)
	// running blacklist-remove with a nonexistent yaml file should print
	// a message and succeed
	require.True(t, result.Success)

	_, shutdownErr := adapter.Shutdown(clientLog)
	require.NoError(t, shutdownErr)

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

func TestDebugConfigInit(t *testing.T) {
	adapter, err := adapter.Create("lxc")
	require.NoError(t, err)

	config, e := client.LoadConfig([]byte(`{"debugConfig":{"resourceLimits": {"cpuLimit": 3, "memoryLimit": 9}}}`))
	require.NoError(t, e)
	config.JobstepID = containerName
	require.NoError(t, adapter.Init(config))
	require.Equal(t, 3, adapter.(*Adapter).container.CpuLimit)
	require.Equal(t, 9, adapter.(*Adapter).container.MemoryLimit)
}

func makeResetFunc(v *int) func() {
	val := *v
	return func() {
		*v = val
	}
}

func TestResourceLimitsInit(t *testing.T) {
	defer makeResetFunc(&cpus)()
	defer makeResetFunc(&memory)()
	ptrto := func(v int) *int { return &v }
	cases := []struct {
		CpusFlag       int
		MemoryFlag     int
		ResourceLimits client.ResourceLimits
		ExpectedCpus   int
		ExpectedMemory int
	}{
		{CpusFlag: 0, MemoryFlag: 0, ResourceLimits: client.ResourceLimits{},
			ExpectedCpus: 0, ExpectedMemory: 0},
		{CpusFlag: 8, MemoryFlag: 8000,
			ResourceLimits: client.ResourceLimits{Cpus: ptrto(4), Memory: ptrto(7000)},
			ExpectedCpus:   4, ExpectedMemory: 7000},
		{CpusFlag: 4, MemoryFlag: 7000,
			ResourceLimits: client.ResourceLimits{Cpus: ptrto(8), Memory: ptrto(8000)},
			ExpectedCpus:   4, ExpectedMemory: 7000},
		{CpusFlag: 0, MemoryFlag: 8000,
			ResourceLimits: client.ResourceLimits{Cpus: ptrto(4)},
			ExpectedCpus:   4, ExpectedMemory: 8000},
	}
	for i, c := range cases {
		cpus, memory = c.CpusFlag, c.MemoryFlag
		adapter, err := adapter.Create("lxc")
		require.NoError(t, err)

		var config client.Config
		config.ResourceLimits = c.ResourceLimits
		config.JobstepID = containerName
		assert.NoError(t, adapter.Init(&config))
		lxcadapter := adapter.(*Adapter)
		assert.Equal(t, c.ExpectedCpus, lxcadapter.container.CpuLimit, "CpuLimit (case %v: %+v)", i, c)
		assert.Equal(t, c.ExpectedMemory, lxcadapter.container.MemoryLimit, "MemoryLimit (case %v: %+v)", i, c)
	}
}
