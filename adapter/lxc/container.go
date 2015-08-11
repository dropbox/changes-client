// +build linux lxc

package lxcadapter

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common/lockfile"
	"gopkg.in/lxc/go-lxc.v2"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const lockTimeout = 1 * time.Hour

type Container struct {
	Release        string
	Arch           string
	Dist           string
	Snapshot       string
	S3Bucket       string
	Name           string
	PreLaunch      string
	PostLaunch     string
	OutputSnapshot string
	MemoryLimit    int
	CpuLimit       int
	BindMounts     []*BindMount
	// Valid values: xz, lz4. These are also used as the file extensions
	// for the rootfs tarballs
	Compression string
	lxc         *lxc.Container
	Executor    *Executor
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

func (c *Container) UploadFile(srcFile string, dstFile string) error {
	rootedDstFile := filepath.Join(c.RootFs(), strings.TrimLeft(dstFile, "/"))
	log.Printf("[lxc] Uploading: %s", rootedDstFile)

	// link isn't flexible enough if /var is not on the same
	// device as root, but our files are small shell scripts.
	return exec.Command("cp", "-r", srcFile, rootedDstFile).Run()
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
		fmt.Println("Cannot initialize lock: %s", err)
		return nil, err
	}

	startTime := time.Now()
	for {
		err = lock.TryLock()
		if err != nil {
			if time.Since(startTime) > lockTimeout {
				return nil, err
			}

			if err == lockfile.ErrBusy {
				fmt.Println(fmt.Sprintf("Lock \"%v\" is busy - retrying in 3 seconds", lock))
				time.Sleep(3 * time.Second)
				continue
			}
		} else {
			return lock, nil
		}
	}
}

func (c *Container) launchOverlayContainer(clientLog *client.Log) error {
	var base *lxc.Container

	clientLog.Writeln(fmt.Sprintf("==> Acquiring lock on container: %s", c.Snapshot))
	lock, err := c.acquireLock(c.Snapshot)
	if err != nil {
		return err
	}
	defer func() {
		clientLog.Writeln(fmt.Sprintf("==> Releasing lock on container: %s", c.Snapshot))
		lock.Unlock()
	}()

	log.Print("[lxc] Checking for cached snapshot")

	if c.snapshotIsCached(c.Snapshot) == false {
		c.ensureImageCached(c.Snapshot, clientLog)

		template := "download"
		if c.Compression != "xz" {
			template = fmt.Sprintf("download-%s", c.Compression)
		}

		clientLog.Writeln(fmt.Sprintf("==> Creating new base container: %s", c.Snapshot))
		clientLog.Writeln(fmt.Sprintf("      Template: %s", template))
		clientLog.Writeln(fmt.Sprintf("      Arch:     %s", c.Arch))
		clientLog.Writeln(fmt.Sprintf("      Distro:   %s", c.Dist))
		clientLog.Writeln(fmt.Sprintf("      Release:  %s", c.Release))
		clientLog.Writeln("    (grab a coffee, this could take a while)")

		start := time.Now()

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
		clientLog.Writeln(fmt.Sprintf("==> Base container online in %s", time.Since(start)))
	} else {
		clientLog.Writeln(fmt.Sprintf("==> Launching existing base container: %s", c.Snapshot))
		log.Print("[lxc] Creating base container")

		start := time.Now()
		base, err = lxc.NewContainer(c.Snapshot, lxc.DefaultConfigPath())
		if err != nil {
			return err
		}
		defer lxc.Release(base)
		clientLog.Writeln(fmt.Sprintf("==> Base container online in %s", time.Since(start)))
	}

	clientLog.Writeln(fmt.Sprintf("==> Clearing lxc cache for base container: %s", c.Snapshot))
	c.removeCachedImage()

	// XXX There must be some odd race condition here as doing `return base.Clone` causes
	// go-lxc to die with a nil-pointer but assigning it to a variable and then returning
	// the variable doesn't. If in the future we see the error again adding a sleep
	// for 0.1 seconds may resolve it (going on the assumption that this part is race-y)
	clientLog.Writeln(fmt.Sprintf("==> Creating overlay container: %s", c.Name))
	err = base.Clone(c.Name, lxc.CloneOptions{
		KeepName: true,
		Snapshot: true,
		Backend:  lxc.Overlayfs,
	})
	if err == nil {
		clientLog.Writeln(fmt.Sprintf("==> Created overlay container:% s", c.Name))
	}
	return err
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
func (c *Container) launchContainer(clientLog *client.Log) error {
	var err error

	c.Executor.Clean()

	if c.Snapshot != "" {
		err := c.launchOverlayContainer(clientLog)
		if err != nil {
			return err
		}
	} else {
		log.Print("[lxc] Creating new container")
		base, err := lxc.NewContainer(c.Name, lxc.DefaultConfigPath())
		base.SetVerbosity(lxc.Quiet)
		if err != nil {
			return err
		}
		defer lxc.Release(base)

		clientLog.Writeln(fmt.Sprintf("==> Creating container: %s", c.Name))
		err = base.Create(lxc.TemplateOptions{
			Template: c.Dist,
			Arch:     c.Arch,
			Release:  c.Release,
		})
		if err != nil {
			return err
		}
		clientLog.Writeln(fmt.Sprintf("==> Created container: %s", c.Name))
	}

	c.lxc, err = lxc.NewContainer(c.Name, lxc.DefaultConfigPath())
	if err != nil {
		return err
	}
	c.lxc.SetVerbosity(lxc.Quiet)

	c.Executor.Register(c.Name)

	if c.PreLaunch != "" {
		log.Print("[lxc] Running pre-launch script")
		err = c.runPreLaunch(clientLog)
		if err != nil {
			return err
		}
	}

	log.Print("[lxc] Configuring container options")
	// More or less disable apparmor
	c.lxc.SetConfigItem("lxc.aa_profile", "unconfined")
	// Allow loop/squashfs in container
	// TODO(dcramer): lxc package doesnt support append, however SetConfigItem seems to append
	c.lxc.SetConfigItem("lxc.cgroup.devices.allow", "c 10:137 rwm")
	c.lxc.SetConfigItem("lxc.cgroup.devices.allow", "b 6:* rwm")

	c.lxc.SetConfigItem("lxc.utsname", fmt.Sprintf("%s-build", c.Name))

	// the default value for cpu_shares is 1024, so we make a soft assumption
	// that we can just magnifiy the value based on the number of cpus we're requesting
	// but it doesnt actually mean we'll get that many cpus
	// http://www.mjmwired.net/kernel/Documentation/scheduler/sched-design-CFS.txt
	if c.CpuLimit != 0 {
		c.lxc.SetCgroupItem("cpu.shares", string(c.CpuLimit*1024))
	}

	// http://www.mjmwired.net/kernel/Documentation/cgroups/memory.txt
	if c.MemoryLimit != 0 {
		c.lxc.SetCgroupItem("memory.limit_in_bytes", string(c.MemoryLimit))
	}

	// Enable autodev: https://wiki.archlinux.org/index.php/Lxc-systemd
	c.lxc.SetConfigItem("lxc.autodev", "1")
	c.lxc.SetConfigItem("lxc.pts", "1024")
	c.lxc.SetConfigItem("lxc.kmsg", "0")

	if c.BindMounts != nil {
		for _, mount := range c.BindMounts {
			c.lxc.SetConfigItem("lxc.mount.entry", mount.Format())
		}
	}

	clientLog.Writeln("==> Waiting for container to be ready")

	log.Print("[lxc] Starting the container")
	err = c.lxc.Start()
	if err != nil {
		return err
	}

	log.Print("[lxc] Waiting for container to startup networking")
	_, err = c.lxc.WaitIPAddresses(30 * time.Second)
	if err != nil {
		return err
	}

	return nil
}

// Takes care of the entire launch process as opposed to up to the
// start of the launch process (above). After the container
// starts up networking at the end of launchContainer,
// we install certificates, update the apt-get cache,
// set up sudoers, and finally run the post-launch script
// which will provision the system (and depends on the completion
// of the above three tasks).
func (c *Container) Launch(clientLog *client.Log) error {
	var err error

	err = c.launchContainer(clientLog)
	if err != nil {
		return err
	}

	// If we aren't using a snapshot then we need to install these,
	// but the apt-get update is expensive so we don't do this
	// if we come from a snapshot.
	if c.Snapshot == "" {
		log.Print("[lxc] Installing ca-certificates")
		cw := NewLxcCommand([]string{"apt-get", "update", "-y", "--fix-missing"}, "root")
		_, err = cw.Run(false, clientLog, c.lxc)
		if err != nil {
			return err
		}
		cw = NewLxcCommand([]string{"apt-get", "install", "-y", "--force-yes", "ca-certificates"}, "root")
		_, err = cw.Run(false, clientLog, c.lxc)
		if err != nil {
			return err
		}

		log.Print("[lxc] Setting up sudoers")
		err = c.setupSudoers()
		if err != nil {
			return err
		}
	}

	if c.PostLaunch != "" {
		log.Print("[lxc] Running post-launch script")
		err = c.runPostLaunch(clientLog)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Container) Stop() error {
	if c.lxc.Running() {
		log.Print("[lxc] Stopping container")
		err := c.lxc.Stop()
		if err != nil {
			return err
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
	var err error

	if c.lxc == nil {
		return nil
	}

	defer lxc.Release(c.lxc)

	c.Stop()

	if c.lxc.Defined() {
		log.Print("[lxc] Destroying container")
		err = c.lxc.Destroy()
		if err != nil {
			return err
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
	dstFile := fmt.Sprintf("/tmp/script-%s", randString(10))

	log.Printf("[lxc] Writing local script %s to %s", cmd.Path, dstFile)

	err := c.UploadFile(cmd.Path, dstFile)
	if err != nil {
		return nil, err
	}

	cw := NewLxcCommand([]string{"chmod", "0755", dstFile}, "root")
	_, err = cw.Run(false, clientLog, c.lxc)
	if err != nil {
		return nil, err
	}

	cw = &LxcCommand{
		Args: []string{dstFile},
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
	return os.RemoveAll(filepath.Join("/var/cache/lxc/download", c.getImagePath(c.Snapshot)))
}

// To avoid complexity of having a sort-of public host, and to ensure we
// can just instead easily store images on S3 (or similar) we attempt to
// sync images in a similar fashion to the LXC image downloader. This means
// that when we attempt to run the image, the download will look for our
// existing cache (that we've correctly populated) and just reference the
// image from there.
func (c *Container) ensureImageCached(snapshot string, clientLog *client.Log) error {
	var err error

	relPath := c.getImagePath(snapshot)
	localPath := filepath.Join("/var/cache/lxc/download", relPath)

	// list of files required to avoid network hit
	fileList := []string{fmt.Sprintf("rootfs.tar.%s", c.Compression), "config", "snapshot_id"}

	var missingFiles bool = false
	for n := range fileList {
		if _, err = os.Stat(filepath.Join(localPath, fileList[n])); os.IsNotExist(err) {
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

	err = os.MkdirAll(localPath, 0755)
	if err != nil {
		return err
	}

	remotePath := fmt.Sprintf("s3://%s/%s", c.S3Bucket, relPath)

	clientLog.Writeln(fmt.Sprintf("==> Downloading image %s", snapshot))
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", "--quiet", remotePath, localPath}, "", []string{
		"HOME=/root",
	})

	start := time.Now()
	result, err := cw.Run(false, clientLog)
	dur := time.Since(start)

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New("Failed downloading image")
	}

	clientLog.Writeln(fmt.Sprintf("==> Image downloaded in %s", dur))

	return nil
}

// Creates the rootfs tarball and all other metadata that the lxc-download
// template expects. This allows us to "act" like an image that the lxc-download
// template would download, but in fact is something entirely different that just
// needs to be treated similarly. The download template expects images to be stored
// on some sort of official server (not s3), but uses cached images when available.
// The image we are creating is to be used as a cached image for the download template.
func (c *Container) CreateImage(snapshot string, clientLog *client.Log) error {
	var err error

	err = c.Stop()
	if err != nil {
		return err
	}

	dest := filepath.Join("/var/cache/lxc/download", c.getImagePath(snapshot))
	clientLog.Writeln(fmt.Sprintf("==> Saving snapshot to %s", dest))
	start := time.Now()

	os.MkdirAll(dest, 0755)

	err = c.createImageMetadata(dest, clientLog)
	if err != nil {
		return err
	}

	err = c.createImageSnapshotID(dest, clientLog)
	if err != nil {
		return err
	}

	err = c.createImageRootFs(dest, clientLog)
	if err != nil {
		return err
	}

	clientLog.Writeln(fmt.Sprintf("==> Snapshot created in %s", time.Since(start)))

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

	err = f.Chmod(0440)
	if err != nil {
		return err
	}
	return nil
}

// Compresses the root of the filesystem into the desired compressed tarball.
// The compression here can vary based on flags.
func (c *Container) createImageRootFs(snapshotPath string, clientLog *client.Log) error {
	rootFsTxz := filepath.Join(snapshotPath, fmt.Sprintf("rootfs.tar.%s", c.Compression))

	clientLog.Writeln(fmt.Sprintf("==> Creating rootfs.tar.%s", c.Compression))

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
	localPath := filepath.Join("/var/cache/lxc/download", relPath)
	remotePath := fmt.Sprintf("s3://%s/%s", c.S3Bucket, relPath)

	clientLog.Writeln(fmt.Sprintf("==> Uploading image %s", snapshot))
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
	clientLog.Writeln(fmt.Sprintf("==> Image uploaded in %s", dur))

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
	preEnv := []string{fmt.Sprintf("LXC_ROOTFS=%s", c.RootFs()), fmt.Sprintf("LXC_NAME=%s", c.Name)}
	cw := client.NewCmdWrapper([]string{c.PreLaunch}, "", preEnv)
	result, err := cw.Run(false, clientLog)
	if err != nil {
		return err
	}

	if !result.Success {
		return errors.New("Post-launch script failed")
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
	cw := &LxcCommand{
		Args: []string{c.PostLaunch},
		User: "root",
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
