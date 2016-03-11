package blacklist

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/dropbox/changes-client/common/scopedlogger"
)

type changesLocalConfig struct {
	RemoveBlacklistFiles bool     `yaml:"build.remove-blacklisted-files"`
	FileBlacklist        []string `yaml:"build.file-blacklist"`
}

func parseYaml(filename string) (changesLocalConfig, error) {
	var config changesLocalConfig
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	return config, err
}

var blacklistLog = scopedlogger.ScopedLogger{Scope: "blacklist"}

// RemoveBlacklistedFiles parses the given yaml file and removes any blacklisted files in the yaml file from rootDir
func RemoveBlacklistedFiles(rootDir string, yamlFile string) error {
	if _, err := os.Stat(yamlFile); os.IsNotExist(err) {
		// non-existent yaml file isn't an error
		blacklistLog.Printf("Project config doesn't exist. Not removing any files.")
		return nil
	}

	config, err := parseYaml(yamlFile)
	if err != nil {
		return err
	}

	if !config.RemoveBlacklistFiles {
		blacklistLog.Printf("Build not configured to remove blacklisted files")
		return nil
	}

	blacklist := config.FileBlacklist
	if len(blacklist) == 0 {
		blacklistLog.Printf("No blacklist entries.")
		return nil
	}

	walkStart := time.Now()
	total := 0
	var matches []string
	visit := func(path string, f os.FileInfo, err error) error {
		// error visiting this path
		if err != nil {
			blacklistLog.Printf("Error walking path %s: %s", path, err)
			return err
		}
		relpath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		total++
		for _, pattern := range blacklist {
			if m, e := fnMatch(pattern, relpath); e != nil {
				return e
			} else if m {
				matches = append(matches, path)
				break
			}
		}
		return nil
	}

	if err := filepath.Walk(rootDir, visit); err != nil {
		return err
	}

	blacklistLog.Printf("Examined %v files in %s", total, time.Since(walkStart))
	blacklistLog.Printf("Removing %d files", len(matches))
	for _, match := range matches {
		if fi, e := os.Stat(match); e != nil {
			// don't error if file doesn't exist (we might've e.g. removed it's underlying directory)
			if !os.IsNotExist(e) {
				return e
			}
		} else if fi.IsDir() {
			if e := os.RemoveAll(match); e != nil {
				return e
			}
		} else {
			if e := os.Remove(match); e != nil {
				return e
			}
		}
	}
	blacklistLog.Printf("Success")
	return nil
}
