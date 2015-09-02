package taggederr_test

import (
	"errors"
	"github.com/dropbox/changes-client/common/taggederr"
	"reflect"
	"testing"
)

func TestMsg(t *testing.T) {
	e := taggederr.Wrap(errors.New("ERROR")).
		AddTag("severity", "hilarity").
		AddTag("death", "smoochy")
	expected := "[death=smoochy,severity=hilarity]: ERROR"
	if msg := e.Error(); msg != expected {
		t.Errorf("Got %v, expected %v", msg, expected)
	}
}

func TestFmtMsg(t *testing.T) {
	e := taggederr.Newf("Experienced %d sadnesses", 4)
	expected := "Experienced 4 sadnesses"
	if msg := e.Error(); msg != expected {
		t.Errorf("Got %v, expected %v", msg, expected)
	}
}

func TestGetTags(t *testing.T) {
	e := taggederr.New("ERROR")
	if len(e.GetTags()) != 0 {
		t.Error("Expected empty tags")
	}
	e2 := e.AddTag("a", "1").AddTag("b", "2")
	expected := map[string]string{
		"a": "1",
		"b": "2",
	}
	if !reflect.DeepEqual(e2.GetTags(), expected) {
		t.Errorf("Expected %v, got %v", expected, e2.GetTags())
	}
}

func TestTaggedErrSafeReuse(t *testing.T) {
	common := taggederr.New("Hello world")
	te1 := common.
		AddTag("name", "me").
		AddTag("cat", "food")
	te2 := common.
		AddTag("name", "you").
		AddTag("dog", "food")
	if te1.Error() == te2.Error() {
		t.Error("Messages shouldn't be the same")
	}
}
