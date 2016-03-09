// +build linux lxc
// +build !nolxc

package lxcadapter

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common/lockfile"
	"github.com/dropbox/changes-client/common/taggederr"
	"gopkg.in/lxc/go-lxc.v2"
)

// The directory in which the container can access files that are copied into
// the host's container.InputMountSource. This is how the container should
// access external files rather than copying them directly to the container.
const containerInputDirectory = "/var/changes/input"

// These binaries are mounted in the container (at /var/changes/input) and can
// thus be accessed as commands to run inside the container (e.g. for our
// current use, removing blacklisted files in the container).
// The binary name must exist on the host machine and be accessible from $PATH
var mountedBinaries = [...]string{"blacklist-remove"}

const lockTimeout = 1 * time.Hour

type Container struct {
	Release        string
	Arch           string
	Dist           string
	Snapshot       string
	S3Bucket       string
	Name           string
	PreLaunch      string
	preLaunchEnv   map[string]string
	PostLaunch     string
	postLaunchEnv  map[string]string
	OutputSnapshot string
	MemoryLimit    int
	CpuLimit       int
	BindMounts     []*BindMount
	// directory we should copy files into to make them accessible to the container.
	InputMountSource string
	// Valid values: xz, lz4. These are also used as the file extensions
	// for the rootfs tarballs
	Compression string
	// Local path to directory of cached images. This determines where
	// images are downloaded to and where we look for them.
	// Required.
	ImageCacheDir string
	lxc           *lxc.Container
	Executor      *Executor
}

type BindMount struct {
	Source  string // include trailing slash
	Dest    string // no trailing slash
	Options string // comma separated, fstab style
}

func ParseBindMount(str string) (*BindMount, error) {
	split := strings.SplitN(str, ":", 3)
	if len(split) != 3 {
		return nil, fmt.Errorf("Invalid bind mount: %s", str)
	}
	return &BindMount{
		Source:  split[0],
		Dest:    split[1],
		Options: split[2],
	}, nil
}

func (b *BindMount) Format() string {
	return fmt.Sprintf("%s %s none bind,%s", b.Source, b.Dest, b.Options)
}

// UploadFile uploads the given srcFile (should be a full path) to the
// container. Uses a bind mount--the file will then be available in the
// container at /var/changes/input/`dstFilename`
func (c *Container) UploadFile(srcFile string, dstFilename string) error {
	mountDstFile := filepath.Join(c.InputMountSource, dstFilename)
	log.Printf("[lxc] Uploading: %s", mountDstFile)

	// link isn't flexible enough if /var is not on the same
	// device as root, but our files are small shell scripts.
	if out, err := exec.Command("cp", "-r", srcFile, mountDstFile).CombinedOutput(); err != nil {
		log.Printf("[lxc] Error uploading file: %s", out)
		return err
	}
	if err := os.Chmod(mountDstFile, 0755); err != nil {
		return err
	}
	return nil
}

func (c *Container) RootFs() string {
	if c.lxc == nil {
		panic("Container not available")
	}
	// May be real path or overlayfs:base-dir:delta-dir
	// TODO(dcramer): confirm this is actually split how we expect it
	bits := strings.Split(c.lxc.ConfigItem("lxc.rootfs")[0], ":")
	return bits[len(bits)-1]
}

func (c *Container) acquireLock(name string) (*lockfile.Lockfile, error) {
	lock, err := lockfile.New(fmt.Sprintf("/tmp/lxc-%s.lock", name))
	if err != nil {
		log.Printf("Cannot initialize lock: %s", err)
		return nil, err
	}

	startTime := time.Now()
	for {
		if err := lock.TryLock(); err != nil {
			if time.Since(startTime) > lockTimeout {
				return nil, err
			}

			if err == lockfile.ErrBusy {
				log.Printf(`Lock "%v" is busy - retrying in 3 seconds`, lock)
				time.Sleep(3 * time.Second)
				continue
			}
		} else {
			return lock, nil
		}
	}
}

// Returns the compression type (xz or lz4) of the container's cached image.
// The second return value indicates success in determining the type.
func (c *Container) getImageCompressionType() (string, bool) {
	localPath := filepath.Join(c.ImageCacheDir, c.getImagePath(c.Snapshot))

	for _, compressionType := range []string{"xz", "lz4"} {
		fileName := "rootfs.tar." + compressionType
		if _, err := os.Stat(filepath.Join(localPath, fileName)); err == nil {
			return compressionType, true
		}
	}
	return "", false
}

func (c *Container) launchOverlayContainer(clientLog *client.Log, metrics client.Metrics) error {
	var base *lxc.Container

	clientLog.Printf("==> Acquiring lock on container: %s", c.Snapshot)
	lock, err := c.acquireLock(c.Snapshot)
	if err != nil {
		return err
	}
	defer func() {
		clientLog.Printf("==> Releasing lock on container: %s", c.Snapshot)
		lock.Unlock()
	}()

	log.Print("[lxc] Checking for cached snapshot")

	if c.snapshotIsCached(c.Snapshot) == false {
		if err := c.ensureImageCached(c.Snapshot, clientLog, metrics); err != nil {
			return err
		}

		template := "download"
		if compressionType, ok := c.getImageCompressionType(); !ok {
			return errors.New("Failed to determine compression type of cached image.")
		} else if compressionType != "xz" {
			template = fmt.Sprintf("download-%s", compressionType)
		}

		clientLog.Printf("==> Creating new base container: %s", c.Snapshot)
		clientLog.Printf("      Template: %s", template)
		clientLog.Printf("      Arch:     %s", c.Arch)
		clientLog.Printf("      Distro:   %s", c.Dist)
		clientLog.Printf("      Release:  %s", c.Release)
		clientLog.Printf("    (grab a coffee, this could take a while)")

		timer := metrics.StartTimer()

		base, err = lxc.NewContainer(c.Snapshot, lxc.DefaultConfigPath())
		if err != nil {
			return err
		}
		defer lxc.Release(base)
		log.Print("[lxc] Creating base container")
		// We can't use Arch/Dist/Release/Variant for anything except
		// for the "download" template, so we specify them manually. However,
		// we can't use extraargs to specify arch/dist/release because the
		// lxc go bindings are lame. (Arch/Distro/Release are all required
		// to be passed, but for consistency we just pass all of them in the
		// case that we are using the download template)
		if template == "download" {
			err = base.Create(lxc.TemplateOptions{
				Template:   "download",
				Arch:       c.Arch,
				Distro:     c.Dist,
				Release:    c.Release,
				Variant:    c.Snapshot,
				ForceCache: true,
			})
		} else {
			err = base.Create(lxc.TemplateOptions{
				Template: template,
				ExtraArgs: []string{
					"--arch", c.Arch,
					"--dist", c.Dist,
					"--release", c.Release,
					"--variant", c.Snapshot,
					"--force-cache",
				},
			})
		}
		if err != nil {
			return err
		}
		timer.Record("baseContainerCreationTime")
	} else {
		clientLog.Printf("==> Launching existing base container: %s", c.Snapshot)
		log.Print("[lxc] Creating base container")

		timer := metrics.StartTimer()
		base, err = lxc.NewContainer(c.Snapshot, lxc.DefaultConfigPath())
		if err != nil {
			return err
		}
		defer lxc.Release(base)
		timer.Record("existingBaseContainerCreationTime")
	}

	clientLog.Printf("==> Clearing lxc cache for base container: %s", c.Snapshot)
	c.removeCachedImage()

	defer metrics.StartTimer().Record("overlayContainerCreationTime")

	// XXX There must be some odd race condition here as doing `return base.Clone` causes
	// go-lxc to die with a nil-pointer but assigning it to a variable and then returning
	// the variable doesn't. If in the future we see the error again adding a sleep
	// for 0.1 seconds may resolve it (going on the assumption that this part is race-y)
	clientLog.Printf("==> Creating overlay container: %s", c.Name)
	err = base.Clone(c.Name, lxc.CloneOptions{
		KeepName: true,
		Snapshot: true,
		Backend:  lxc.Overlayfs,
	})
	if err == nil {
		clientLog.Printf("==> Created overlay container: %s", c.Name)
	}
	return err
}

type configItem struct {
	Name, Value string
}

func (ci configItem) Set(l *lxc.Container) error {
	if e := l.SetConfigItem(ci.Name, ci.Value); e != nil {
		return fmt.Errorf("SetConfigItem(%q, %q) failed: %s", ci.Name, ci.Value, e)
	}
	return nil
}

type configSetter interface {
	Set(*lxc.Container) error
}

// In this phase we actually launch the container that the tests
// will be run in.
//
// There are essentially four cases:
//  - we aren't using a snapshot
//  - we are using a snapshot but don't have it cached
//  - we are using a cached snapshot but don't have a base container
//  - we are using a cached snapshot and have a base container
//
// The first case is clearly different from the latter three, and indeed
// the process it follows is rather different because it doesn't use the
// same template. In the first case, without a snapshot, we use the
// ubuntu template (see /usr/share/lxc/templates) to create a container,
// and destroy it at the end. Only the first run of this will be extremely
// slow to create the container itself, but after that it will be faster,
// although it still must pay a heavy cost for provisioning.
//
// For the latter three cases, it follows a general process saving work
// where work doesn't have to be done. First, it checks if we have a
// snapshot base container or not. If we do, it clones it using overlayfs
// and then starts the resulting container; the existing base container
// is not modified.
//
// If we don't have a base container, then it checks for a compressed
// tarball of the filesystem. This is either are .tar.xz or .tar.lz4
// and the compression must match what compression changes-client is
// being used for. If this file doesn't exist, the client fetches it
// from the given s3 bucket in a folder qualified by its arch, dist,
// release, and snapshot id.
//
// Once we have guaranteed that we have a snapshot image, the snapshot
// image is loaded using the "download" template (or a variant for it
// if we are using the faster lz4 compression). This template will
// require the image already to be cached - as it can't download it
// like normal - so we use --force-cached as a template option. Once
// the base container is up, we proceed as normal, and we leave the
// base container alive so that future runs are fast.
//
// Once the container is started, we mount the container and perform
// basic configurations as well as run the pre-launch script.
func (c *Container) launchContainer(clientLog *client.Log, metrics client.Metrics) error {

	c.Executor.Clean()

	timer := metrics.StartTimer()
	if c.Snapshot != "" {
		if err := c.launchOverlayContainer(clientLog, metrics); err != nil {
			return err
		}
		timer.Record("snapshotContainerCreationTime")
	} else {
		log.Print("[lxc] Creating new container")
		base, err := lxc.NewContainer(c.Name, lxc.DefaultConfigPath())
		base.SetVerbosity(lxc.Quiet)
		if err != nil {
			return err
		}
		defer lxc.Release(base)

		clientLog.Printf("==> Creating container: %s", c.Name)
		if err := base.Create(lxc.TemplateOptions{
			Template: c.Dist,
			Arch:     c.Arch,
			Release:  c.Release,
		}); err != nil {
			return err
		}
		clientLog.Printf("==> Created container: %s", c.Name)
		timer.Record("noSnapshotContainerCreationTime")
	}
	defer metrics.StartTimer().Record("containerStartupTime")

	if newcont, err := lxc.NewContainer(c.Name, lxc.DefaultConfigPath()); err != nil {
		return err
	} else {
		c.lxc = newcont
	}
	c.lxc.SetVerbosity(lxc.Quiet)

	c.Executor.Register(c.Name)

	if c.PreLaunch != "" {
		log.Print("[lxc] Running pre-launch script")
		if err := c.runPreLaunch(clientLog); err != nil {
			return err
		}
	}

	log.Print("[lxc] Configuring container options")
	for _, cs := range c.getConfigSetters() {
		if e := cs.Set(c.lxc); e != nil {
			return e
		}
	}

	clientLog.Printf("==> Waiting for container to be ready")

	log.Print("[lxc] Starting the container")
	if err := c.lxc.Start(); err != nil {
		return err
	}

	log.Print("[lxc] Waiting for container to startup networking")
	beforeNetwork := time.Now()
	if _, err := c.lxc.WaitIPAddresses(30 * time.Second); err != nil {
		return err
	}
	log.Printf("[lxc] Networking up after %s", time.Since(beforeNetwork))

	return nil
}

// getConfigSetters returns the configSetters that should be applied to the container before starting.
func (c *Container) getConfigSetters() []configSetter {
	result := []configSetter{
		// More or less disable apparmor
		configItem{"lxc.aa_profile", "unconfined"},
		// Allow loop/squashfs in container
		configItem{"lxc.cgroup.devices.allow", "b 7:* rwm"},
		configItem{"lxc.cgroup.devices.allow", "c 10:137 rwm"},
		configItem{"lxc.utsname", c.Name + "-build"},

		// Enable autodev: https://wiki.archlinux.org/index.php/Lxc-systemd
		configItem{"lxc.autodev", "1"},
		configItem{"lxc.pts", "1024"},
		configItem{"lxc.kmsg", "0"},
		configItem{"lxc.seccomp", ""},
	}

	if c.CpuLimit != 0 {
		// CFS Bandwidth Control Reference: https://www.kernel.org/doc/Documentation/scheduler/sched-bwc.txt
		// https://access.redhat.com/documentation/en-US/Red_Hat_Enterprise_Linux/6/html/Resource_Management_Guide/sec-cpu.html
		//
		// Every 100ms,
		result = append(result, configItem{"lxc.cgroup.cpu.cfs_period_us", strconv.Itoa(100000)})

		// Allow up to CpuLimit * 100ms of CPU usage for the container.
		result = append(result, configItem{"lxc.cgroup.cpu.cfs_quota_us", strconv.Itoa(c.CpuLimit * 100000)})
	}

	// http://www.mjmwired.net/kernel/Documentation/cgroups/memory.txt
	if c.MemoryLimit != 0 {
		result = append(result, configItem{"lxc.cgroup.memory.limit_in_bytes", strconv.Itoa(c.MemoryLimit) + "M"})
	}

	for _, mount := range c.BindMounts {
		result = append(result, configItem{"lxc.mount.entry", mount.Format()})
	}

	return result
}

// Takes care of the entire launch process as opposed to up to the
// start of the launch process (above). After the container
// starts up networking at the end of launchContainer,
// we install certificates, update the apt-get cache,
// set up sudoers, and finally run the post-launch script
// which will provision the system (and depends on the completion
// of the above three tasks).
func (c *Container) Launch(clientLog *client.Log) (client.Metrics, error) {
	metrics := client.Metrics{}
	timer := metrics.StartTimer()
	if err := c.launchContainer(clientLog, metrics); err != nil {
		return metrics, err
	}
	timer.Record("containerLaunchTime")

	currentConfig := make(map[string][]string)
	for _, k := range c.lxc.ConfigKeys() {
		currentConfig[k] = c.lxc.ConfigItem(k)
	}
	if configJSON, err := json.MarshalIndent(currentConfig, "", "   "); err != nil {
		// Should be impossible.
		panic(err)
	} else {
		log.Println("Container config: ", string(configJSON))
	}

	defer metrics.StartTimer().Record("postLaunchTime")
	if c.Snapshot == "" {
		log.Print("[lxc] Setting up sudoers")
		if err := c.setupSudoers(); err != nil {
			return metrics, err
		}
	}

	if c.PostLaunch != "" {
		log.Print("[lxc] Running post-launch script")
		if err := c.runPostLaunch(clientLog); err != nil {
			return metrics, err
		}
	}
	for _, binary := range mountedBinaries {
		// Makes this binary available from within the container
		binaryPath, err := exec.LookPath(binary)
		if err != nil {
			return metrics, err
		}
		if err := c.UploadFile(binaryPath, binary); err != nil {
			return metrics, err
		}
	}
	return metrics, nil
}

func parseInt64(str string) (int64, error) {
	return strconv.ParseInt(str, 10, 64)
}

func parseCgroupStats(cgroupStats []string, metrics client.Metrics) error {
	// cgroupStats is a string slice with ["nr_periods XX", "nr_throttled XX", "throttled_time XX"]
	if periods, err := parseInt64(strings.TrimPrefix(cgroupStats[0], "nr_periods ")); err != nil {
		return err
	} else {
		metrics["cpuPeriods"] = float64(periods)
	}

	if throttled, err := parseInt64(strings.TrimPrefix(cgroupStats[1], "nr_throttled ")); err != nil {
		return err
	} else {
		metrics["throttledPeriods"] = float64(throttled)
	}

	if throttledTimeNanos, err := parseInt64(strings.TrimPrefix(cgroupStats[2], "throttled_time ")); err != nil {
		return err
	} else {
		metrics.SetDuration("throttledTime", time.Duration(throttledTimeNanos)*time.Nanosecond)
	}

	return nil
}

// Report container resource information from the container to the infra log.
// Should be called while the container is running, but after work is done.
func (c *Container) logResourceUsageStats() client.Metrics {
	metrics := client.Metrics{}

	if usage, err := c.lxc.BlkioUsage(); err != nil {
		log.Printf("[lxc] Failed to get disk IO: %s", err)
	} else {
		metrics["blkioUsageBytes"] = float64(usage)
		log.Printf("[lxc] Total disk IO: %s", usage)
	}

	if total, err := c.lxc.CPUTime(); err != nil {
		log.Printf("[lxc] Failed to get CPU time: %s", err)
	} else {
		metrics.SetDuration("cpuTime", total)
		log.Printf("[lxc] Total CPU time: %s", total)
	}

	if cgroupStats := c.lxc.CgroupItem("cpu.stat"); cgroupStats == nil {
		log.Printf("[lxc] Failed to get Cgroup stats")
	} else {
		log.Printf("[lxc] Cgroup stats: %v", cgroupStats)
		if err := parseCgroupStats(cgroupStats, metrics); err != nil {
			log.Printf("[lxc] Failed to parse Cgroup stats: %s", err)
		}
	}

	// Documentation for memory.failcnt at
	// https://access.redhat.com/documentation/en-US/Red_Hat_Enterprise_Linux/6/html/Resource_Management_Guide/sec-memory.html
	if memoryFailures := c.lxc.CgroupItem("memory.failcnt"); len(memoryFailures) == 0 {
		log.Printf("[lxc] Failed to get memory failures")
	} else {
		log.Printf("[lxc] Max memory usage: %v bytes", memoryFailures[0])
		if b, err := parseInt64(memoryFailures[0]); err != nil {
			log.Printf("[lxc] Error parsing memory failures")
		} else {
			if b != 0 {
				log.Printf("[lxc] Detected %d memory failures - failures may be caused by OOM kill", b)
			}
			metrics["memoryFailures"] = float64(b)
		}
	}

	if maxUsageInBytes := c.lxc.CgroupItem("memory.max_usage_in_bytes"); len(maxUsageInBytes) == 0 {
		log.Printf("[lxc] Failed to get max memory usage")
	} else {
		log.Printf("[lxc] Max memory usage: %v bytes", maxUsageInBytes[0])
		if b, err := parseInt64(maxUsageInBytes[0]); err != nil {
			log.Printf("[lxc] Error parsing max memory usage")
		} else {
			metrics["maxMemoryUsageBytes"] = float64(b)
		}
	}

	times, e := c.lxc.CPUTimePerCPU()
	if e != nil {
		log.Printf("[lxc] Failed to get per-CPU stats: %s", e)
		return metrics
	}
	var cpuids []int
	for k := range times {
		cpuids = append(cpuids, k)
	}
	sort.Ints(cpuids)
	for _, id := range cpuids {
		log.Printf("[lxc] CPU %d: %s", id, times[id])
	}

	return metrics
}

func (c *Container) Stop() error {
	if c.lxc.Running() {
		log.Print("[lxc] Stopping container")
		err := c.lxc.Stop()
		if err != nil {
			return tagged(err).AddTag("action", "Stop")
		}
	}
	if c.lxc.Running() {
		return errors.New("Container is still running")
	}
	return nil
}

// Destroys the container. We ensure that this is called as long
// as we don't have --keep-container as an option or we don't get
// SIGKILLed (which happens when Jenkins aborts builds, so we use
// the executor mechanism to handle destroying containers instead
// in that scenario - however, this is delayed until the start of
// the next build)
func (c *Container) Destroy() error {
	// Destroy must operate idempotently
	if c.lxc == nil {
		return nil
	}

	defer lxc.Release(c.lxc)

	// We don't return on error here, because we're only stopping
	// to be polite.
	_ = c.Stop()

	if c.lxc.Defined() {
		log.Print("[lxc] Destroying container")
		if err := c.lxc.Destroy(); err != nil {
			return tagged(err).AddTag("action", "Destroy")
		}
	}
	if c.lxc.Defined() {
		return errors.New("Container was not destroyed")
	}
	c.Executor.Deregister()
	return nil
}

// Our CI scripts run as the user ubuntu but may need access
// to root. In order to get around this, we just give all users
// in the container access to root as this is no less dangerous.
func (c *Container) setupSudoers() error {
	sudoersPath := filepath.Join(c.RootFs(), "etc", "sudoers")
	f, err := os.Create(sudoersPath)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("Defaults    env_reset\n")
	f.WriteString("Defaults    !requiretty\n\n")
	f.WriteString("# Allow all sudoers.\n")
	f.WriteString("ALL  ALL=(ALL) NOPASSWD:ALL\n")

	err = f.Chmod(0440)
	if err != nil {
		return err
	}

	return nil
}

func randString(n int) string {
	const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return string(bytes)
}

// Runs a command in a container. This "uploads" the command to the container,
// essentially copying the command from the host to the container filesystem,
// and then runs the new temporary file, capturing output.
func (c *Container) RunCommandInContainer(cmd *client.Command, clientLog *client.Log, user string) (*client.CommandResult, error) {
	dstFilename := fmt.Sprintf("script-%s", randString(10))

	log.Printf("[lxc] Writing local script %s to %s", cmd.Path, dstFilename)

	err := c.UploadFile(cmd.Path, dstFilename)
	if err != nil {
		return nil, err
	}

	mountedFile := filepath.Join(containerInputDirectory, dstFilename)

	cw := &LxcCommand{
		Args: []string{mountedFile},
		User: user,
		Cwd:  cmd.Cwd,
		Env:  cmd.Env,
	}
	return cw.Run(cmd.CaptureOutput, clientLog, c.lxc)
}

// Gets the image path associated with a specific snapshot.
func (c *Container) getImagePath(snapshot string) string {
	return filepath.Join("ubuntu", c.Release, "amd64", snapshot)
}

// Checks to see if an existing snapshot is cached. This does not
// refer to the tarball but rather to the base container.
func (c *Container) snapshotIsCached(snapshot string) bool {
	for _, name := range lxc.ContainerNames() {
		if snapshot == name {
			return true
		}
	}
	return false
}

// Remove the /var/cache/lxc directory for a snapshot in order to
// save disk space as snapshots can be rather large and once
// the base container is live we have no need to store the old
// tarball.
func (c *Container) removeCachedImage() error {
	// Note that this won't fail if the cache doesn't exist, which
	// is the desireed behavior
	return os.RemoveAll(filepath.Join(c.ImageCacheDir, c.getImagePath(c.Snapshot)))
}

// To avoid complexity of having a sort-of public host, and to ensure we
// can just instead easily store images on S3 (or similar) we attempt to
// sync images in a similar fashion to the LXC image downloader. This means
// that when we attempt to run the image, the download will look for our
// existing cache (that we've correctly populated) and just reference the
// image from there.
func (c *Container) ensureImageCached(snapshot string, clientLog *client.Log, metrics client.Metrics) error {
	defer metrics.StartTimer().Record("snapshotImageDownloadTime")

	relPath := c.getImagePath(snapshot)
	localPath := filepath.Join(c.ImageCacheDir, relPath)

	// list of files required to avoid network hit
	fileList := []string{"config", "snapshot_id"}

	missingFiles := false

	// If we weren't able to determine the compression type, it's probably because we can't
	// find the image file.
	if _, ok := c.getImageCompressionType(); !ok {
		missingFiles = true
	}
	for n := range fileList {
		if _, err := os.Stat(filepath.Join(localPath, fileList[n])); os.IsNotExist(err) {
			missingFiles = true
			break
		}
	}
	if !missingFiles {
		return nil
	}

	if c.S3Bucket == "" {
		return errors.New("Unable to find cached image, and no S3 bucket defined.")
	}

	if err := os.MkdirAll(localPath, 0755); err != nil {
		return err
	}

	remotePath := fmt.Sprintf("s3://%s/%s", c.S3Bucket, relPath)

	clientLog.Printf("==> Downloading image %s", snapshot)
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", "--quiet", remotePath, localPath}, "", []string{
		"HOME=/root",
	})

	result, err := cw.Run(false, clientLog)

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New("Failed downloading image")
	}

	return nil
}

// Creates the rootfs tarball and all other metadata that the lxc-download
// template expects. This allows us to "act" like an image that the lxc-download
// template would download, but in fact is something entirely different that just
// needs to be treated similarly. The download template expects images to be stored
// on some sort of official server (not s3), but uses cached images when available.
// The image we are creating is to be used as a cached image for the download template.
func (c *Container) CreateImage(snapshot string, clientLog *client.Log) error {
	if err := c.Stop(); err != nil {
		return err
	}

	dest := filepath.Join(c.ImageCacheDir, c.getImagePath(snapshot))
	clientLog.Printf("==> Saving snapshot to %s", dest)
	start := time.Now()

	os.MkdirAll(dest, 0755)

	if err := c.createImageMetadata(dest, clientLog); err != nil {
		return err
	}

	if err := c.createImageSnapshotID(dest, clientLog); err != nil {
		return err
	}

	if err := c.createImageRootFs(dest, clientLog); err != nil {
		return err
	}

	clientLog.Printf("==> Snapshot created in %s", time.Since(start))

	return nil
}

func (c *Container) createImageMetadata(snapshotPath string, clientLog *client.Log) error {
	metadataPath := filepath.Join(snapshotPath, "config")
	f, err := os.Create(metadataPath)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("lxc.include = LXC_TEMPLATE_CONFIG/ubuntu.common.conf\n")
	f.WriteString("lxc.arch = x86_64\n")

	return f.Chmod(0440)
}

// Compresses the root of the filesystem into the desired compressed tarball.
// The compression here can vary based on flags.
func (c *Container) createImageRootFs(snapshotPath string, clientLog *client.Log) error {
	rootFsTxz := filepath.Join(snapshotPath, fmt.Sprintf("rootfs.tar.%s", c.Compression))

	clientLog.Printf("==> Creating rootfs.tar.%s", c.Compression)

	var cw *client.CmdWrapper
	if c.Compression == "xz" {
		cw = client.NewCmdWrapper([]string{"tar", "-Jcf", rootFsTxz, "-C", c.RootFs(), "."}, "", []string{})
	} else {
		cw = client.NewCmdWrapper([]string{"tar", "-cf", rootFsTxz, "-I", "lz4", "-C", c.RootFs(), "."}, "", []string{})
	}
	result, err := cw.Run(false, clientLog)

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New(fmt.Sprintf("Failed creating rootfs.tar.%s", c.Compression))
	}

	return nil
}

func (c *Container) createImageSnapshotID(snapshotPath string, clientLog *client.Log) error {
	metadataPath := filepath.Join(snapshotPath, "snapshot_id")
	f, err := os.Create(metadataPath)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf("%s\n", c.Name))

	err = f.Chmod(0440)
	if err != nil {
		return err
	}
	return nil
}

// Uploads a snapshot outcome to an s3 bucket, at the same path that
// changes-client will expect to download it from. The snapshot itself
// is just a tarball of the rootfs of the container - compressed with
// either xz for high compression or lz4 for raw speed.
func (c *Container) UploadImage(snapshot string, clientLog *client.Log) error {
	relPath := c.getImagePath(snapshot)
	localPath := filepath.Join(c.ImageCacheDir, relPath)
	remotePath := fmt.Sprintf("s3://%s/%s", c.S3Bucket, relPath)

	clientLog.Printf("==> Uploading image %s", snapshot)
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", "--quiet", localPath, remotePath}, "", []string{})

	start := time.Now()
	result, err := cw.Run(false, clientLog)
	dur := time.Since(start)

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New("Failed uploading image")
	}
	clientLog.Printf("==> Image uploaded in %s", dur)

	return nil
}

// Should we keep the container around?
//
// Currently we decide to keep the container only if the file
// /home/ubuntu/KEEP-CONTAINER exists within the container.
func (c *Container) ShouldKeep() bool {
	fullPath := filepath.Join(c.RootFs(), "/home/ubuntu/KEEP-CONTAINER")
	_, err := os.Stat(fullPath)
	return err == nil
}

// Runs the prelaunch script which is essentially responsible for setting up
// the environment for the post-launch script. It runs within the host
// environment with the container mounted at LXC_ROOTFS. Runs as the user
// that changes-client runs as (usually root).
func (c *Container) runPreLaunch(clientLog *client.Log) error {
	env := []string{"LXC_ROOTFS=" + c.RootFs(), "LXC_NAME=" + c.Name}
	for k, v := range c.preLaunchEnv {
		env = append(env, k+"="+v)
	}
	cw := client.NewCmdWrapper([]string{c.PreLaunch}, "", env)
	result, err := cw.Run(false, clientLog)
	if err != nil {
		return err
	}

	if !result.Success {
		return errors.New("Pre-launch script failed")
	}

	return nil
}

// Runs the post launch script which is responsible for the provisioning
// of the base container. This is generally costly only for non-snapshotted
// builds as the base container is relatively static.
//
// This runs within the container environment, not the host. Runs as the user
// 'ubuntu'.
func (c *Container) runPostLaunch(clientLog *client.Log) error {
	var env []string
	for k, v := range c.postLaunchEnv {
		env = append(env, k+"="+v)
	}
	cw := &LxcCommand{
		Args: []string{c.PostLaunch},
		User: "root",
		Env:  env,
	}
	result, err := cw.Run(false, clientLog, c.lxc)
	if err != nil {
		return err
	}

	if !result.Success {
		return errors.New("Post-launch script failed")
	}

	return nil
}

func tagged(e error) taggederr.TaggedErr {
	return taggederr.Wrap(e).AddTag("adapter", "lxc")
}
