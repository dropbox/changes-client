// +build linux lxc

package lxcadapter

import (
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/dropbox/changes-client/shared/lockfile"
	"github.com/dropbox/changes-client/shared/runner"
	"gopkg.in/lxc/go-lxc.v2"
	"log"
	"os"
	"path"
	"strings"
	"time"
)

var (
	lockTimeout = int64((1 * time.Hour).Seconds())
)

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
	lxc            *lxc.Container
}

func (c *Container) UploadFile(srcFile string, dstFile string) error {
	log.Printf("[lxc] Uploading: %s", path.Join(c.RootFs(), strings.TrimLeft(dstFile, "/")))
	return os.Link(srcFile, path.Join(c.RootFs(), strings.TrimLeft(dstFile, "/")))
}

func (c *Container) RootFs() string {
	// May be real path or overlayfs:base-dir:delta-dir
	// TODO(dcramer): confirm this is actually split how we expect it
	bits := strings.Split(c.lxc.ConfigItem("lxc.rootfs")[0], ":")
	return bits[len(bits)-1]
}

func (c *Container) acquireLock(name string) (*lockfile.Lockfile, error) {
	var currentTime int64

	lock, err := lockfile.New(fmt.Sprintf("/tmp/lxc-%s.lock", name))
	if err != nil {
		fmt.Println("Cannot initialize lock: %s", err)
		return nil, err
	}

	startTime := time.Now().Unix()
	for {
		err = lock.TryLock()
		if err != nil {
			currentTime = time.Now().Unix()
			if currentTime-startTime > lockTimeout {
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

func (c *Container) launchContainer(clientLog *runner.Log) error {
	var err error
	var base *lxc.Container
	var lockName string

	if c.Snapshot != "" {
		lockName = c.Snapshot
	} else {
		lockName = c.Name
	}

	// It's possible for multiple clients to compete w/ downloading and then
	// defining the container so on the first failure we simply try again
	clientLog.Writeln(fmt.Sprintf("==> Acquiring lock on container: %s", lockName))
	lock, err := c.acquireLock(lockName)
	if err != nil {
		return err
	}
	defer func() {
		clientLog.Writeln(fmt.Sprintf("==> Releasing lock on container: %s", lockName))
		lock.Unlock()
	}()

	if c.Snapshot != "" {
		log.Print("[lxc] Checking for cached snapshot")

		if c.snapshotIsCached(c.Snapshot) == false {
			c.ensureImageCached(c.Snapshot, clientLog)

			clientLog.Writeln(fmt.Sprintf("==> Creating new base container: %s", c.Snapshot))
			clientLog.Writeln(fmt.Sprintf("      Arch:    %s", c.Arch))
			clientLog.Writeln(fmt.Sprintf("      Distro:  %s", c.Dist))
			clientLog.Writeln(fmt.Sprintf("      Release: %s", c.Release))
			clientLog.Writeln("    (grab a coffee, this could take a while)")

			start := time.Now().Unix()

			base, err = lxc.NewContainer(c.Snapshot, lxc.DefaultConfigPath())
			defer lxc.Release(base)

			log.Print("[lxc] Creating base container")
			err = base.Create(lxc.TemplateOptions{
				Template:   "download",
				Arch:       c.Arch,
				Distro:     c.Dist,
				Release:    c.Release,
				Variant:    c.Snapshot,
				ForceCache: true,
			})
			stop := time.Now().Unix()
			if err != nil {
				return err
			}
			clientLog.Writeln(fmt.Sprintf("==> Base container online in %ds", stop-start))
		} else {
			clientLog.Writeln(fmt.Sprintf("==> Launching existing base container: %s", c.Snapshot))
			log.Print("[lxc] Creating base container")

			start := time.Now().Unix()
			base, err = lxc.NewContainer(c.Snapshot, lxc.DefaultConfigPath())
			stop := time.Now().Unix()
			if err != nil {
				return err
			}
			defer lxc.Release(base)
			clientLog.Writeln(fmt.Sprintf("==> Base container online in %ds", stop-start))
		}

		clientLog.Writeln(fmt.Sprintf("==> Creating overlay container: %s", c.Name))
		err = base.Clone(c.Name, lxc.CloneOptions{
			KeepName: true,
			Snapshot: true,
			Backend:  lxc.Overlayfs,
		})
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
	}

	c.lxc, err = lxc.NewContainer(c.Name, lxc.DefaultConfigPath())
	c.lxc.SetVerbosity(lxc.Quiet)

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

func (c *Container) Launch(clientLog *runner.Log) error {
	var err error

	err = c.launchContainer(clientLog)
	if err != nil {
		return err
	}

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
	return nil
}

func (c *Container) setupSudoers() error {
	sudoersPath := path.Join(c.RootFs(), "etc", "sudoers")
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

func (c *Container) RunCommandInContainer(cmd *runner.Command, clientLog *runner.Log, user string) (*runner.CommandResult, error) {
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

func (c *Container) getImagePath(snapshot string) string {
	return fmt.Sprintf("ubuntu/%s/amd64/%s", c.Release, snapshot)
}

func (c *Container) snapshotIsCached(snapshot string) bool {
	for _, name := range lxc.ContainerNames() {
		if snapshot == name {
			return true
		}
	}
	return false
}

// To avoid complexity of having a sort-of public host, and to ensure we
// can just instead easily store images on S3 (or similar) we attempt to
// sync images in a similar fashion to the LXC image downloader. This means
// that when we attempt to run the image, the download will look for our
// existing cache (that we've correctly populated) and just reference the
// image from there.
func (c *Container) ensureImageCached(snapshot string, clientLog *runner.Log) error {
	var err error

	relPath := c.getImagePath(snapshot)
	localPath := fmt.Sprintf("/var/cache/lxc/download/%s", relPath)

	// list of files required to avoid network hit
	fileList := []string{"rootfs.tar.xz", "config", "snapshot_id"}

	var missingFiles bool = false
	for n := range fileList {
		if _, err = os.Stat(path.Join(localPath, fileList[n])); os.IsNotExist(err) {
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
	cw := runner.NewCmdWrapper([]string{"aws", "s3", "sync", "--quiet", remotePath, localPath}, "", []string{
		"HOME=/root",
	})

	start := time.Now().Unix()
	result, err := cw.Run(false, clientLog)
	stop := time.Now().Unix()

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New("Failed downloading image")
	}

	clientLog.Writeln(fmt.Sprintf("==> Image downloaded in %ds", stop-start))

	return nil
}

func (c *Container) CreateImage(snapshot string, clientLog *runner.Log) error {
	var err error

	err = c.Stop()
	if err != nil {
		return err
	}

	dest := fmt.Sprintf("/var/cache/lxc/download/%s", c.getImagePath(snapshot))
	clientLog.Writeln(fmt.Sprintf("==> Saving snapshot to %s", dest))
	start := time.Now().Unix()

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

	stop := time.Now().Unix()
	clientLog.Writeln(fmt.Sprintf("==> Snapshot created in %ds", stop-start))

	return nil
}

func (c *Container) createImageMetadata(snapshotPath string, clientLog *runner.Log) error {
	metadataPath := path.Join(snapshotPath, "config")
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

func (c *Container) createImageRootFs(snapshotPath string, clientLog *runner.Log) error {
	rootFsTxz := path.Join(snapshotPath, "rootfs.tar.xz")

	clientLog.Writeln("==> Creating rootfs.tar.xz")

	cw := runner.NewCmdWrapper([]string{"tar", "-Jcf", rootFsTxz, "-C", c.RootFs(), "."}, "", []string{})
	result, err := cw.Run(false, clientLog)

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New("Failed creating rootfs.tar.xz")
	}

	return nil
}

func (c *Container) createImageSnapshotID(snapshotPath string, clientLog *runner.Log) error {
	metadataPath := path.Join(snapshotPath, "snapshot_id")
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

func (c *Container) UploadImage(snapshot string, clientLog *runner.Log) error {
	relPath := c.getImagePath(snapshot)
	localPath := fmt.Sprintf("/var/cache/lxc/download/%s", relPath)
	remotePath := fmt.Sprintf("s3://%s/%s", c.S3Bucket, relPath)

	clientLog.Writeln(fmt.Sprintf("==> Uploading image %s", snapshot))
	// TODO(dcramer): verify env is passed correctly here
	cw := runner.NewCmdWrapper([]string{"aws", "s3", "sync", "--quiet", localPath, remotePath}, "", []string{})

	start := time.Now().Unix()
	result, err := cw.Run(false, clientLog)
	stop := time.Now().Unix()

	if err != nil {
		return err
	}
	if !result.Success {
		return errors.New("Failed uploading image")
	}
	clientLog.Writeln(fmt.Sprintf("==> Image uploaded in %ds", stop-start))

	return nil
}

func (c *Container) runPreLaunch(clientLog *runner.Log) error {
	preEnv := []string{fmt.Sprintf("LXC_ROOTFS=%s", c.RootFs()), fmt.Sprintf("LXC_NAME=%s", c.Name)}
	cw := runner.NewCmdWrapper([]string{c.PreLaunch}, "", preEnv)
	result, err := cw.Run(false, clientLog)
	if err != nil {
		return err
	}

	if !result.Success {
		return errors.New("Post-launch script failed")
	}

	return nil
}

func (c *Container) runPostLaunch(clientLog *runner.Log) error {
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
