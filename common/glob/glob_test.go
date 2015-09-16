package glob

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func newfile(root, name string) error {
	path := filepath.Join(root, name)
	if filepath.Dir(path) != root {
		if e := os.MkdirAll(filepath.Dir(path), 0777); e != nil {
			return e
		}
	}
	return ioutil.WriteFile(path, []byte("test"), 0777)
}

func newfiles(root string, names ...string) error {
	for _, n := range names {
		if e := newfile(root, n); e != nil {
			return e
		}
	}
	return nil
}

func TestGlobRegular(t *testing.T) {
	dirname, e := ioutil.TempDir("", "globtree")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dirname)
	if e := newfiles(dirname, "base.xml", "foo/test.xml", "coverage.xml/ohmy.txt", "bar/ohsnap.xml"); e != nil {
		t.Fatal(e)
	}
	// TODO: Move this to a build tagged file once we have builders that don't support Mkfifo.
	if e := syscall.Mkfifo(filepath.Join(dirname, "special.xml"), 0777); e != nil {
		t.Fatal(e)
	}
	matches, skipped, e := GlobTreeRegular(dirname, []string{"*.xml"})
	if e != nil {
		t.Fatal(e)
	}
	expected := []string{"base.xml", "foo/test.xml", "bar/ohsnap.xml"}
	for _, ex := range expected {
		p := filepath.Join(dirname, ex)
		if !contains(matches, p) {
			t.Errorf("Expected %q, but not found", ex)
		}
	}

	unexpected := []string{"coverage.xml", "special.xml"}
	for _, uex := range unexpected {
		p := filepath.Join(dirname, uex)
		if contains(matches, p) {
			t.Errorf("Didn't expected %q, but matched", uex)
		}
		if !contains(skipped, p) {
			t.Errorf("Expected %q to be skipped, but wasn't", p)
		}
	}
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if s == v {
			return true
		}
	}
	return false
}
