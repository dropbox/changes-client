package sentry

import (
	"errors"
	"github.com/dropbox/changes-client/common/taggederr"
	"reflect"
	"testing"
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
