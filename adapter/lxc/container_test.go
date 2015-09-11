// +build linux lxc

package lxcadapter

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "gopkg.in/check.v1"
)

func TestContainer(t *testing.T) { TestingT(t) }

type ContainerSuite struct{}

var _ = Suite(&ContainerSuite{})

func ensureFileExists(path string) {
	if _, err := os.Stat(path); err != nil {
		err = ioutil.WriteFile(path, []byte("test\n"), 644)
		if err != nil {
			panic("Failed to create file")
		}
	}
}

func ensureFileDoesNotExist(path string) {
	if _, err := os.Stat(path); err == nil {
		err = os.Remove(path)
		if err != nil {
			panic("Failed to remove file")
		}
	}
}

func (s *ContainerSuite) TestGetImageCompressionType(c *C) {
	cacheDir, err := ioutil.TempDir("/tmp", "container_test")
	c.Assert(err, IsNil)
	container := &Container{
		Name:          containerName,
		ImageCacheDir: cacheDir,
		Snapshot:      "test_snapshot",
	}

	imagePath := filepath.Join(cacheDir, container.getImagePath("test_snapshot"))
	err = os.MkdirAll(imagePath, 755)
	c.Assert(err, IsNil)
	defer os.RemoveAll(cacheDir)

	lz4Path := filepath.Join(imagePath, "rootfs.tar.lz4")
	xzPath := filepath.Join(imagePath, "rootfs.tar.xz")

	// Test case where the image doesn't exist
	ensureFileDoesNotExist(lz4Path)
	ensureFileDoesNotExist(xzPath)
	_, ok := container.getImageCompressionType()
	c.Assert(ok, Equals, false)

	// Test lz4 case
	ensureFileExists(lz4Path)
	compressionType, ok := container.getImageCompressionType()
	c.Assert(ok, Equals, true)
	c.Assert(compressionType, Equals, "lz4")

	// Test xz case
	ensureFileDoesNotExist(lz4Path)
	ensureFileExists(xzPath)
	compressionType, ok = container.getImageCompressionType()
	c.Assert(ok, Equals, true)
	c.Assert(compressionType, Equals, "xz")
}
