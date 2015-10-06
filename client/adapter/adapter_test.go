// +build linux lxc

package adapter

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFormatUUID(t *testing.T) {
	res := FormatUUID("a6f70a68e4384cf68bcc2dd1a44b8554")
	assert.Equal(t, res, "a6f70a68-e438-4cf6-8bcc-2dd1a44b8554")
}
