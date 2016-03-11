// +build linux lxc

package lxcadapter

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestGetImageCompressionType(t *testing.T) {
	cacheDir, err := ioutil.TempDir("/tmp", "container_test")
	require.Nil(t, err)
	container := &Container{
		Name:          containerName,
		ImageCacheDir: cacheDir,
		Snapshot:      "test_snapshot",
	}

	imagePath := filepath.Join(cacheDir, container.getImagePath("test_snapshot"))
	require.NoError(t, os.MkdirAll(imagePath, 755))
	defer os.RemoveAll(cacheDir)

	lz4Path := filepath.Join(imagePath, "rootfs.tar.lz4")
	xzPath := filepath.Join(imagePath, "rootfs.tar.xz")

	// Test case where the image doesn't exist
	ensureFileDoesNotExist(lz4Path)
	ensureFileDoesNotExist(xzPath)
	_, ok := container.getImageCompressionType()
	require.False(t, ok)

	// Test lz4 case
	ensureFileExists(lz4Path)
	compressionType, ok := container.getImageCompressionType()
	require.True(t, ok)
	require.Equal(t, compressionType, "lz4")

	// Test xz case
	ensureFileDoesNotExist(lz4Path)
	ensureFileExists(xzPath)
	compressionType, ok = container.getImageCompressionType()
	assert.True(t, ok)
	assert.Equal(t, compressionType, "xz")
}
