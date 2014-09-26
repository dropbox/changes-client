package lxc

import (
	"fmt"
    "github.com/dropbox/changes-client/client"
    "os"
    "path"
	"time"
)

type Container struct {
	name     string
	running  bool
	rootfs   string
	release  string
	s3Bucket string
}

func NewContainer(name string) *Container {
	return &Container{
		name: name,
	}
}

func (c *Container) UploadFile(srcFile string, dstFile string) error {
    return os.Link(srcFile, path.Join(c.rootfs, dstFile))
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

// To avoid complexity of having a sort-of public host, and to ensure we
// can just instead easily store images on S3 (or similar) we attempt to
// sync images in a similar fashion to the LXC image downloader. This means
// that when we attempt to run the image, the download will look for our
// existing cache (that we've correctly populated) and just reference the
// image from there.
func (c *Container) ensureImageCached(snapshot string, log *client.Log) error {
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

	log.Writeln(fmt.Sprintf("==> Downloading image %s", snapshot))
	start := time.Now().Unix()
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", remotePath, localPath}, "", []string{})
	_, err = cw.Run(false, log)
	if err != nil {
		return err
	}
	stop := time.Now().Unix()
	log.Writeln(fmt.Sprintf("==> Image downloaded in %ds", stop-start*100))

	return nil
}

func (c *Container) uploadImage(snapshot string, log *client.Log) error {
	relPath := c.getImagePath(snapshot)
	localPath := fmt.Sprintf("/var/cache/lxc/download/%s", relPath)
	remotePath := fmt.Sprintf("s3://%s/%s", c.s3Bucket, relPath)

	start := time.Now().Unix()
	log.Writeln(fmt.Sprintf("==> Uploading image %s", snapshot))
	// TODO(dcramer): verify env is passed correctly here
	cw := client.NewCmdWrapper([]string{"aws", "s3", "sync", localPath, remotePath}, "", []string{})
	_, err := cw.Run(false, log)
	if err != nil {
		return err
	}
	stop := time.Now().Unix()
	log.Writeln(fmt.Sprintf("==> Image uploaded in %ds", stop-start*100))

    return nil
}

func (c *Container) GenerateCommand(command []string, user string) []string {
	// TODO(dcramer):
    // homeDir := c.getHomeDir(user)
	// env = {
	//     # TODO(dcramer): HOME is pretty hacky here
	//     'USER': user,
	//     'HOME': home_dir,
	//     'PWD': cwd,
	//     'DEBIAN_FRONTEND': 'noninteractive',
	//     'LXC_NAME': self.name,
	//     'HOST_HOSTNAME': socket.gethostname(),
	//     'PATH': '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',
	// }
	//     if env:
	//         new_env.update(env)

	result := []string{"lxc-run", "-n", c.name, "--", "sudo", "-EHu", user}
    result = append(result, command...)
    return result
}
