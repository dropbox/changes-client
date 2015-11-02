// +build linux lxc
// +build !nolxc

package lxcadapter

import (
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"
	"time"

	autil "github.com/dropbox/changes-client/adapter"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/common/sentry"
	"gopkg.in/lxc/go-lxc.v2"
)

type Adapter struct {
	config         *client.Config
	container      *Container
	artifactSource string
}

func (a *Adapter) Init(config *client.Config) error {
	snapshot := config.Snapshot.ID
	if snapshot != "" {
		if s3Bucket == "" {
			log.Print("[lxc] WARNING: s3bucket is not defined, snapshot ignored")
			snapshot = ""
		} else {
			snapshot = adapter.FormatUUID(snapshot)
		}
	}

	// In reality our goal is to make a switch completely to lz4, but we need to retain
	// compatibility with mesos builds for now, so we default to "xz" and also try
	// to not uncleanly die if its set to a weird value, also setting it to "xz."
	if compression != "xz" && compression != "lz4" {
		compression = "xz"
		log.Printf("[lxc] Warning: invalid compression %s, defaulting to lzma", compression)
	}
	executor := &Executor{
		Name:      executorName,
		Directory: executorPath,
	}

	var mounts []*BindMount
	if bindMounts != "" {
		mountStrings := strings.Split(bindMounts, ",")
		mounts = make([]*BindMount, len(mountStrings))
		for ind, ms := range mountStrings {
			var err error
			mounts[ind], err = ParseBindMount(ms)
			if err != nil {
				return err
			}
		}
	}

	mergeLimits := func(v int, other *int) int {
		if other != nil {
			if v == 0 || *other < v {
				return *other
			}
		}
		return v
	}

	cpuLimit := mergeLimits(cpus, config.ResourceLimits.Cpus)
	memoryLimit := mergeLimits(memory, config.ResourceLimits.Memory)

	container := &Container{
		Name:           config.JobstepID,
		Arch:           arch,
		Dist:           dist,
		Release:        release,
		PreLaunch:      preLaunch,
		PostLaunch:     postLaunch,
		Snapshot:       snapshot,
		OutputSnapshot: config.ExpectedSnapshot.ID,
		// TODO(dcramer):  Move S3 logic into core engine
		S3Bucket:      s3Bucket,
		MemoryLimit:   memoryLimit,
		CpuLimit:      cpuLimit,
		Compression:   compression,
		Executor:      executor,
		BindMounts:    mounts,
		ImageCacheDir: "/var/cache/lxc/download",
	}

	// DebugConfig limits override standard config.
	var limits struct {
		CpuLimit    *int
		MemoryLimit *int
	}
	if ok, err := config.GetDebugConfig("resourceLimits", &limits); err != nil {
		log.Printf("[lxc] %s", err)
	} else if ok {
		if limits.CpuLimit != nil {
			container.CpuLimit = *limits.CpuLimit
		}
		if limits.MemoryLimit != nil {
			container.MemoryLimit = *limits.MemoryLimit
		}
	}

	if _, err := config.GetDebugConfig("prelaunch_env", &container.preLaunchEnv); err != nil {
		log.Printf("[lxc] Failed to parse prelaunch_env: %s", err)
	}
	if _, err := config.GetDebugConfig("postlaunch_env", &container.postLaunchEnv); err != nil {
		log.Printf("[lxc] Failed to parse postlaunch_env: %s", err)
	}

	a.config = config
	a.container = container

	return nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(clientLog *client.Log) (client.Metrics, error) {
	clientLog.Printf("LXC version: %s", lxc.Version())
	metrics, err := a.container.Launch(clientLog)
	if err != nil {
		return metrics, err
	}

	containerArtifactSource := "/home/ubuntu"
	if a.config.ArtifactSearchPath != "" {
		containerArtifactSource = a.config.ArtifactSearchPath
	}
	artifactSource, err := filepath.Abs(path.Join(a.container.RootFs(), containerArtifactSource))
	if err == nil {
		a.artifactSource = artifactSource
	}
	return metrics, err
}

// Runs a given command. This may be called multiple times depending
func (a *Adapter) Run(cmd *client.Command, clientLog *client.Log) (*client.CommandResult, error) {
	return a.container.RunCommandInContainer(cmd, clientLog, "ubuntu")
}

// Perform any cleanup actions within the environment.
func (a *Adapter) Shutdown(clientLog *client.Log) error {
	const timeout = 30 * time.Second
	timer := time.AfterFunc(timeout, func() {
		sentry.Message(fmt.Sprintf("Took more than %s to shutdown LXC adapter", timeout), map[string]string{})
	})
	defer timer.Stop()
	a.container.logResourceUsageStats(clientLog)
	if keepContainer || a.container.ShouldKeep() || shouldDebugKeep(clientLog, a.config) {
		a.container.Executor.Deregister()

		// Create a "named executor" which will never get cleaned
		// up by changes-client but allows the outside environment
		// to recognize that this container is still associated
		// with changes-client.
		//
		// This executor has the same name as the container rather
		// than the executor identifier provided by command-line
		// flags. The container name is generally unique as it
		// corresponds to a jobstep, unlike the executor identifier
		// which is defined to not be unique.
		executor := Executor{
			Name:      a.container.Name,
			Directory: a.container.Executor.Directory,
		}
		executor.Register(a.container.Name)
		return nil
	}
	return a.container.Destroy()
}

// Parses debugConfig.lxc_keep_container_end_rfc3339 as an RFC3339 timestamp.
// Example: "2015-10-08T19:31:56Z" or "2015-10-08T12:32:19-07:00"
func shouldDebugKeep(clientLog *client.Log, cfg *client.Config) bool {
	const key = "lxc_keep_container_end_rfc3339"
	var keepEndtime string
	if ok, err := cfg.GetDebugConfig(key, &keepEndtime); err != nil {
		clientLog.Printf("[lxc] %s", err)
		return false
	} else if !ok {
		return false
	}
	endTime, err := time.Parse(time.RFC3339, keepEndtime)
	if err != nil {
		clientLog.Printf("[lxc] Couldn't parse %s %q as time: %s", key, keepEndtime, err)
		return false
	}
	return time.Now().Before(endTime)
}

func (a *Adapter) CaptureSnapshot(outputSnapshot string, clientLog *client.Log) error {
	outputSnapshot = adapter.FormatUUID(outputSnapshot)

	if err := a.container.CreateImage(outputSnapshot, clientLog); err != nil {
		return err
	}

	if a.container.S3Bucket != "" {
		if err := a.container.UploadImage(outputSnapshot, clientLog); err != nil {
			return err
		}
	} else {
		log.Printf("[lxc] warning: cannot upload snapshot, no s3 bucket specified")
	}
	return nil
}

func (a *Adapter) GetRootFs() string {
	return a.container.RootFs()
}

func (a *Adapter) CollectArtifacts(artifacts []string, clientLog *client.Log) ([]string, error) {
	log.Printf("[lxc] Searching for %s in %s", artifacts, a.artifactSource)
	return autil.CollectArtifactsIn(a.artifactSource, artifacts, clientLog)
}

func (a *Adapter) GetArtifactRoot() string {
	return a.artifactSource
}

func New() adapter.Adapter {
	return &Adapter{}
}

func init() {
	adapter.Register("lxc", New)
}
