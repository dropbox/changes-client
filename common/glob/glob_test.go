package glob

import (
	"fmt"
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
	if e := newfiles(dirname, "base.xml", "foo/test.xml", "coverage.xml/ohmy.txt", "bar/ohsnap.xml", "tests.json", "foo/tests.json", "foo/bar/weird.json", "foo/bar/baz/weird.json", "bar/foo/weird.json"); e != nil {
		t.Fatal(e)
	}
	// TODO: Move this to a build tagged file once we have builders that don't support Mkfifo.
	if e := syscall.Mkfifo(filepath.Join(dirname, "special.xml"), 0777); e != nil {
		t.Fatal(e)
	}
	matches, skipped, e := GlobTreeRegular(dirname, []string{"*.xml", "/tests.json", "foo/*/weird.json"})
	if e != nil {
		t.Fatal(e)
	}
	matches = stripPrefix(t, dirname, matches)
	skipped = stripPrefix(t, dirname, skipped)
	expected := []string{"base.xml", "foo/test.xml", "bar/ohsnap.xml", "tests.json", "foo/bar/weird.json"}
	if e := equalAnyOrder(expected, matches); e != nil {
		t.Errorf("GlobTreeRegular had unexpected matches: %s", e)
	}

	shouldSkip := []string{"coverage.xml", "special.xml"}
	if e := equalAnyOrder(shouldSkip, skipped); e != nil {
		t.Errorf("GlobTreeRegular had unexpected skips: %s", e)
	}
}

func stripPrefix(t *testing.T, prefix string, slice []string) []string {
	var strippedArray []string
	for _, elem := range slice {
		if rel, err := filepath.Rel(prefix, elem); err != nil {
			t.Fatal(err)
		} else {
			strippedArray = append(strippedArray, rel)
		}
	}
	return strippedArray
}

func equalAnyOrder(expected, actual []string) error {
	errMsg := ""
	for _, elem := range expected {
		if !contains(actual, elem) {
			errMsg += fmt.Sprintf("Expected %q but not found\n", elem)
		}
	}
	for _, elem := range actual {
		if !contains(expected, elem) {
			errMsg += fmt.Sprintf("Didn't expect %q but it was present\n", elem)
		}
	}
	if errMsg != "" {
		return fmt.Errorf(errMsg)
	}
	return nil
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if s == v {
			return true
		}
	}
	return false
}
