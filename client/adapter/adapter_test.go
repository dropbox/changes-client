// +build linux lxc

package adapter

import (
	"testing"

	. "gopkg.in/check.v1"
)

func TestAdapter(t *testing.T) { TestingT(t) }

type AdapterSuite struct{}

var _ = Suite(&AdapterSuite{})

func (s *AdapterSuite) TestFormatUUID(c *C) {
	res := FormatUUID("a6f70a68e4384cf68bcc2dd1a44b8554")
	c.Assert(res, Equals, "a6f70a68-e438-4cf6-8bcc-2dd1a44b8554")
}
