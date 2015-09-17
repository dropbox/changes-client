// +build linux lxc

package lxcadapter

import (
	"flag"
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

var (
	preLaunch     string
	postLaunch    string
	s3Bucket      string
	release       string
	arch          string
	dist          string
	keepContainer bool
	memory        int
	cpus          int
	compression   string
	executorName  string
	executorPath  string
	bindMounts    string
)

type Adapter struct {
	config         *client.Config
	container      *Container
	artifactSource string
}

func (a *Adapter) Init(config *client.Config) error {
	var snapshot string = config.Snapshot.ID
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
	mounts = nil
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

	container := &Container{
		Name:       config.JobstepID,
		Arch:       arch,
		Dist:       dist,
		Release:    release,
		PreLaunch:  preLaunch,
		PostLaunch: postLaunch,
		Snapshot:   snapshot,
		// TODO(dcramer):  Move S3 logic into core engine
		S3Bucket:      s3Bucket,
		MemoryLimit:   memory,
		CpuLimit:      cpus,
		Compression:   compression,
		Executor:      executor,
		BindMounts:    mounts,
		ImageCacheDir: "/var/cache/lxc/download",
	}

	a.config = config
	a.container = container

	return nil
}

// Prepare the environment for future commands. This is run before any
// commands are processed and is run once.
func (a *Adapter) Prepare(clientLog *client.Log) error {
	clientLog.Writeln("LXC version: " + lxc.Version())
	err := a.container.Launch(clientLog)
	if err != nil {
		return err
	}

	artifactSource := "/home/ubuntu"
	if a.config.ArtifactSearchPath != "" {
		artifactSource = a.config.ArtifactSearchPath
	}
	artifactSource, err = filepath.Abs(path.Join(a.container.RootFs(), strings.TrimLeft(artifactSource, "/")))
	if err != nil {
		return err
	}
	a.artifactSource = artifactSource

	return nil
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
	if keepContainer || a.container.ShouldKeep() {
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

func (a *Adapter) CaptureSnapshot(outputSnapshot string, clientLog *client.Log) error {
	outputSnapshot = adapter.FormatUUID(outputSnapshot)

	err := a.container.CreateImage(outputSnapshot, clientLog)
	if err != nil {
		return err
	}

	if a.container.S3Bucket != "" {
		err = a.container.UploadImage(outputSnapshot, clientLog)
		if err != nil {
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

func New() adapter.Adapter {
	return &Adapter{}
}

func init() {
	flag.StringVar(&preLaunch, "pre-launch", "", "Container pre-launch script")
	flag.StringVar(&postLaunch, "post-launch", "", "Container post-launch script")
	flag.StringVar(&s3Bucket, "s3-bucket", "", "S3 bucket name")
	flag.StringVar(&dist, "dist", "ubuntu", "Linux distribution")
	flag.StringVar(&release, "release", "trusty", "Distribution release")
	flag.StringVar(&arch, "arch", "amd64", "Linux architecture")
	// This is the compression algorithm to be used for creating an image.
	// The decompression used is determined by whether the image has the "xz" or "lz4" extension.
	flag.StringVar(&compression, "compression", "lz4", "compression algorithm (xz,lz4)")
	flag.StringVar(&bindMounts, "bind-mounts", "", "bind mounts. <source>:<dest>:<options>. comma separated.")

	// the executor should have the following properties:
	//  - the maximum distinct values passed to executor is equal to the maximum
	//    number of concurrently running jobs.
	//  - no two changes-client processes should be called with the same
	//    executor name
	//  - if any process is calling changes-client with executor specified, then
	//    all clients should use a specified executor
	//
	// if not all of these features can be met, then executor should not be specified
	// but parallel builds may not work correctly.
	//
	flag.StringVar(&executorName, "executor", "", "Executor (unique runner id)")
	flag.StringVar(&executorPath, "executor-path", "/var/lib/changes-client/executors", "Path to store executors")
	flag.IntVar(&memory, "memory", 0, "Memory limit (in MB)")
	flag.IntVar(&cpus, "cpus", 0, "CPU limit")
	flag.BoolVar(&keepContainer, "keep-container", false, "Do not destroy the container on cleanup")

	adapter.Register("lxc", New)
}
