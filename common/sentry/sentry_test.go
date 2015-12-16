package sentry

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dropbox/changes-client/common/taggederr"
	raven "github.com/getsentry/raven-go"
	"reflect"
	"testing"
	"time"
)

func TestExtractFromTagged(t *testing.T) {
	type M map[string]string
	base := errors.New("Test Error")
	terr := taggederr.Wrap(base).AddTag("x", "1").AddTag("y", "2")
	cases := []struct {
		override map[string]string
		result   map[string]string
	}{
		{override: nil, result: M{"x": "1", "y": "2"}},
		{override: M{}, result: M{"x": "1", "y": "2"}},
		{override: M{"x": "4"}, result: M{"x": "4", "y": "2"}},
		{override: M{"z": "4"}, result: M{"x": "1", "y": "2", "z": "4"}},
	}
	for i, c := range cases {
		// Nil override.
		oute, outt := extractFromTagged(terr, c.override)
		if oute != base {
			t.Errorf("%v: Expected base error, got %v", i, oute)
		}
		if !reflect.DeepEqual(outt, c.result) {
			t.Errorf("%v: Expected %v, got %v", i, c.result, outt)
		}
	}
}

func TestMakePacket(t *testing.T) {
	pkt := makePacket(raven.ERROR, "Hello World", &raven.Message{Message: "Hello %s", Params: []interface{}{"World"}})
	type Wire struct {
		Message  string
		Level    string
		LogEntry struct {
			Message string
			Params  []interface{}
		}
	}
	var out Wire
	if e := json.Unmarshal(pkt.JSON(), &out); e != nil {
		t.Fatal(e)
	}
	if out.Message != "Hello World" {
		t.Errorf("Bad message: %v", out.Message)
	}

	if fmt.Sprintf(out.LogEntry.Message, out.LogEntry.Params...) != "Hello World" {
		t.Errorf("Bad log entry: %#v", out.LogEntry)
	}
}

func TestFmtSanitize(t *testing.T) {
	mkargs := func(args ...interface{}) []interface{} {
		return args
	}
	result := func(fmtstr string, args ...interface{}) *raven.Message {
		return &raven.Message{Message: fmtstr, Params: args}
	}
	cases := []struct {
		Msg    string
		Args   []interface{}
		result *raven.Message
	}{
		{"Hello %s", mkargs("World"), result("Hello %s", "World")},
		{"Hello %q", mkargs("World"), result("Hello %s", `"World"`)},
		{"Hello %v", mkargs("World"), result("Hello %s", "World")},
		{"Hello %v", mkargs(1i), result("Hello %s", "(0+1i)")},
		{"From %s to %s!", mkargs("Justin", "Kelly"), result("From %s to %s!", "Justin", "Kelly")},
		{"Hello %%", mkargs(), result("Hello %%")},
		{"Hello %q", mkargs(), nil},
		{"Ran %v times", mkargs(4), result("Ran %s times", "4")},
		{"Ran %q times", mkargs(4.5), result("Ran %s times", "%!q(float64=4.5)")},
		{"Gone in %s", mkargs(59 * time.Second), result("Gone in %s", "59s")},
		{"If %d then %t", mkargs(5, false), nil},
		{"But %%v ", mkargs("what"), nil},
		{"%%%", mkargs(), nil},
		{"Hello there.", mkargs(), result("Hello there.")},
	}

	for i, c := range cases {
		ravenMsg := fmtSanitize(c.Msg, c.Args)
		if !reflect.DeepEqual(ravenMsg, c.result) {
			t.Errorf("%v: Expected: %v but got %v", i, c.result, ravenMsg)
		}
	}
}
