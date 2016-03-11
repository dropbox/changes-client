package blacklist

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/dropbox/changes-client/common/scopedlogger"
	"strings"
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

type blentry struct {
	// The full blacklist pattern.
	full string
	// The prefix of the blacklist pattern containing no meta-characters.
	plainPrefix string
}

type blacklistMatcher struct {
	entries []blentry
}

func newMatcher(entries []string) blacklistMatcher {
	blentries := make([]blentry, 0, len(entries))
	for _, ent := range entries {
		const metaChars = `*[?\`
		plainEnd := strings.IndexAny(ent, metaChars)
		if plainEnd == -1 {
			plainEnd = len(ent)
		}
		blentries = append(blentries, blentry{plainPrefix: ent[:plainEnd], full: ent})
	}
	return blacklistMatcher{blentries}
}

func (blm blacklistMatcher) Match(relpath string) (bool, error) {
	for _, pattern := range blm.entries {
		// Fast-path for the common case (/foo/bar/*); if there isn't
		// a prefix match, fnMatch can't match.
		if !strings.HasPrefix(relpath, pattern.plainPrefix) {
			continue
		}
		if m, e := fnMatch(pattern.full, relpath); e != nil || m {
			return m, e
		}
	}
	return false, nil
}

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

	if len(config.FileBlacklist) == 0 {
		blacklistLog.Printf("No blacklist entries.")
		return nil
	}

	blmatcher := newMatcher(config.FileBlacklist)
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
		if m, e := blmatcher.Match(relpath); e != nil {
			return e
		} else if m {
			matches = append(matches, path)
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
