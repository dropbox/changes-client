package lxcadapter

import (
	"flag"
)

// Flags are stored here so they are available even for non-LXC builds.
// Ideally we'd only supply relevant flags, but an unexpectd flag can cause an early failure
// that's awkward to report reliably.
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
}
