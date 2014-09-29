package lxcadapter

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
	"gopkg.in/lxc/go-lxc.v1"
	"log"
	"os"
	"path"
	"time"
)

type Container struct {
	release    string
	arch       string
	dist       string
	snapshot   string
	s3Bucket   string
	name       string
	preLaunch  string
	postLaunch string
	lxc        *lxc.Container
}

func NewContainer(name string, preLaunch string, postLaunch string) (*Container, error) {
	return &Container{
		name:       name,
		arch:       "amd64",
		dist:       "ubuntu",
		release:    "precise",
		preLaunch:  preLaunch,
		postLaunch: postLaunch,
	}, nil
}

func (c *Container) UploadFile(srcFile string, dstFile string) error {
	return os.Link(srcFile, path.Join(c.RootFs(), dstFile))
}

func (c *Container) RootFs() string {
	// May be real path or overlayfs:base-dir:delta-dir
	// TODO(dcramer): confirm this is actually split how we expect it
	return c.lxc.ConfigItem("lxc.rootfs")[1]
}

func (c *Container) Launch(clientLog *client.Log) error {
	var err error
	var base *lxc.Container

	if c.snapshot != "" {
		log.Print("[lxc] Checking for cached snapshot")
		if c.snapshotIsCached(c.snapshot) == false {
			c.ensureImageCached(c.snapshot, clientLog)

			base, err = lxc.NewContainer(c.snapshot, lxc.DefaultConfigPath())
			if err != nil {
				return err
			}
			defer lxc.PutContainer(base)

			err = base.Create("download", "--arch", c.arch, "--release", c.release,
				"--dist", c.dist, "--variant", c.snapshot)
			if err != nil {
				return err
			}
		} else {
			base, err = lxc.NewContainer(c.snapshot, lxc.DefaultConfigPath())
			if err != nil {
				return err
			}
			defer lxc.PutContainer(base)
		}

		clientLog.Writeln(fmt.Sprintf("==> Overlaying container: %s", c.snapshot))
		flags := lxc.CloneKeepName | lxc.CloneSnapshot
		err = base.CloneUsing(c.name, lxc.Overlayfs, flags)
		if err != nil {
			return err
		}
	} else {
		log.Print("[lxc] Creating new container")
		base, err := lxc.NewContainer(c.name, lxc.DefaultConfigPath())
		base.SetVerbosity(lxc.Quiet)
		if err != nil {
			return err
		}
		defer lxc.PutContainer(base)

		clientLog.Writeln("==> Creating container")
		if os.Geteuid() != 0 {
			err = base.CreateAsUser(c.dist, c.release, c.arch)
		} else {
			err = base.Create(c.dist, "--release", c.release, "--arch", c.arch)
		}
		if err != nil {
			return err
		}
	}

	c.lxc, err = lxc.NewContainer(c.name, lxc.DefaultConfigPath())
	c.lxc.SetVerbosity(lxc.Quiet)
	log.Print("[lxc] Running pre-launch script")
	if c.preLaunch != "" {
		cw := client.NewCmdWrapper([]string{c.preLaunch}, "", []string{})
		_, err = cw.Run(false, clientLog)
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

	clientLog.Writeln("==> Waiting for container to be ready")

	log.Print("[lxc] Starting the container")
	err = c.lxc.Start()
	if err != nil {
		return err
	}

	// TODO(dcramer): there is no timeout in go-lxc, we might need a spin loop
	log.Print("[lxc] Waiting for container to startup networking")

	// TODO(dcramer): add timeout
	sem := make(chan bool)
	go func() {
		for {
			_, err = c.lxc.IPv4Addresses()
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		sem <- true
	}()
	<-sem

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

	log.Print("[lxc] Running post-launch script")
	if c.postLaunch != "" {
		_, err = c.RunLocalScript(c.postLaunch, false, clientLog)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Container) Destroy() error {
	lxc.PutContainer(c.lxc)
	err := c.lxc.Destroy()
	if err != nil {
		return err
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

func (c *Container) RunLocalScript(path string, captureOutput bool, clientLog *client.Log) (*client.CommandResult, error) {
	dstFile := "/tmp/script"
	err := c.UploadFile(path, dstFile)
	if err != nil {
		return nil, err
	}

	cw := NewLxcCommand([]string{"chmod", "0755", dstFile}, "ubuntu")
	_, err = cw.Run(false, clientLog, c.lxc)
	if err != nil {
		return nil, err
	}

	cw = NewLxcCommand([]string{dstFile}, "ubuntu")
	return cw.Run(captureOutput, clientLog, c.lxc)
}

func (c *Container) getHomeDir(user string) string {
	if user == "root" {
		return "/root"
	} else {
		return fmt.Sprintf("/home/%s", user)
	}
}

func (c *Container) getImagePath(snapshot string) string {
	return fmt.Sprintf("ubuntu/%s/amd64/%s", c.release, snapshot)
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
func (c *Container) ensureImageCached(snapshot string, clientLog *client.Log) error {
	relPath := c.getImagePath(snapshot)
	localPath := fmt.Sprintf("/var/cache/lxc/download/%s", relPath)

	// list of files required to avoid network hit
	// TODO(dcramer):
	// fileList := []string{"rootfs.tar.xz", "config", "snapshot_id"}
	// if all(os.path.exists(os.path.join(local_path, f)) for f in file_list):
	//     return

	err := os.MkdirAll(localPath, 0755)
	if err != nil {
		return err
	}

	remotePath := fmt.Sprintf("s3://%s/%s", c.s3Bucket, relPath)

	clientLog.Writeln(fmt.Sprintf("==> Downloading image %s", snapshot))
	start := time.Now().Unix()
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", remotePath, localPath}, "", []string{})
	_, err = cw.Run(false, clientLog)
	if err != nil {
		return err
	}
	stop := time.Now().Unix()
	clientLog.Writeln(fmt.Sprintf("==> Image downloaded in %ds", stop-start*100))

	return nil
}

func (c *Container) uploadImage(snapshot string, clientLog *client.Log) error {
	relPath := c.getImagePath(snapshot)
	localPath := fmt.Sprintf("/var/cache/lxc/download/%s", relPath)
	remotePath := fmt.Sprintf("s3://%s/%s", c.s3Bucket, relPath)

	start := time.Now().Unix()
	clientLog.Writeln(fmt.Sprintf("==> Uploading image %s", snapshot))
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", localPath, remotePath}, "", []string{})
	_, err := cw.Run(false, clientLog)
	if err != nil {
		return err
	}
	stop := time.Now().Unix()
	clientLog.Writeln(fmt.Sprintf("==> Image uploaded in %ds", stop-start*100))

	return nil
}
