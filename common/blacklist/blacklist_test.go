package blacklist

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sameslice(s1 []string, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for idx, elem := range s1 {
		if elem != s2[idx] {
			return false
		}
	}
	return true
}

func makeyaml(path string, remove bool) error {
	template := `
build.remove-blacklisted-files: %t
build.file-blacklist:
    - dir1/*
    - dir2/dir3/*
    - dir2/other.txt
    - dir2/*/baz.py
    - "[!a-z].txt"
    - toplevelfile.txt
    - nonexistent.txt
`
	contents := fmt.Sprintf(template, remove)
	return ioutil.WriteFile(path, []byte(contents), 0777)
}

func newfile(root, name string) error {
	path := filepath.Join(root, name)
	// we use trailing slash to signify a directory
	if strings.HasSuffix(name, "/") {
		if e := os.MkdirAll(path, 0777); e != nil {
			return e
		}
		return nil
	}
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

func TestParseYaml(t *testing.T) {
	dirname, err := ioutil.TempDir("", "parseyaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)
	yamlfile := filepath.Join(dirname, "foo.yaml")
	if err = makeyaml(yamlfile, true); err != nil {
		t.Fatal(err)
	}
	config, err := parseYaml(yamlfile)
	if err != nil {
		t.Fatal(err)
	}
	if !config.RemoveBlacklistFiles {
		t.Error("Config incorrectly read RemoveBlacklistFiles as false")
	}

	expected := []string{"dir1/*", "dir2/dir3/*", "dir2/other.txt", "dir2/*/baz.py", "[!a-z].txt", "toplevelfile.txt", "nonexistent.txt"}
	if !sameslice(config.FileBlacklist, expected) {
		t.Errorf("Config incorrectly parsed blacklisted files. Actual: %v, Expected: %v", config.FileBlacklist, expected)
	}
}

func removeBlacklistFilesHelper(t *testing.T, tempDirName string, remove bool) {
	dirname, err := ioutil.TempDir("", tempDirName)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(dirname)
	defer os.Chdir(cwd)
	yamlfile := filepath.Join(dirname, "foo.yaml")
	if err = makeyaml(yamlfile, remove); err != nil {
		t.Fatal(err)
	}

	dontMatchBlacklist := []string{"dir1/", "dir2/", "dir2/dir3/", "dir2/foo.txt", "foo/toplevelfile.txt", "a.txt"}
	matchBlacklist := []string{"dir1/foo.txt", "dir1/other/", "dir1/other/bar.txt", "dir2/dir3/baz.yaml", "dir2/other.txt", "dir2/foo/bar/baz.py", "0.txt", "toplevelfile.txt"}

	var shouldExist []string
	var shouldntExist []string
	if remove {
		shouldExist = dontMatchBlacklist
		shouldntExist = matchBlacklist
	} else {
		// if yaml file says not to remove blacklisted files everything should still exist
		shouldExist = append(dontMatchBlacklist, matchBlacklist...)
		shouldntExist = []string{}
	}

	if err = newfiles(".", append(shouldExist, shouldntExist...)...); err != nil {
		t.Fatal(err)
	}

	err = RemoveBlacklistedFiles(".", "foo.yaml")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range shouldExist {
		if _, err := os.Stat(file); err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File %s shouldn't have been removed but was", file)
			} else {
				t.Errorf("Error checking existence of %s: %s", file, err)
			}
		}
	}

	for _, file := range shouldntExist {
		if _, err := os.Stat(file); err != nil && !os.IsNotExist(err) {
			t.Errorf("Error checking non-existence of %s: %s", file, err)
		} else if err == nil {
			t.Errorf("File %s should have been removed but wasn't", file)
		}
	}
}

func TestRemoveBlacklistFilesTrue(t *testing.T) {
	removeBlacklistFilesHelper(t, "removeblacklistfilestrue", true)
}

func TestRemoveBlacklistFilesFalse(t *testing.T) {
	removeBlacklistFilesHelper(t, "removeblacklistfilesfalse", false)
}

func TestBlacklistNoYamlFile(t *testing.T) {
	dirname, err := ioutil.TempDir("", "blacklistnoyamlfile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dirname)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	os.Chdir(dirname)
	defer os.Chdir(cwd)
	err = RemoveBlacklistedFiles(".", "bar.yaml")
	if err != nil {
		t.Errorf("Encountered error when yaml file didn't exist: %s", err)
	}
}
