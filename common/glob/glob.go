// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glob

import (
	"os"
	"path/filepath"
	"strings"
)

// GlobTreeRegular walks root looking for regular (non-dir, non-device) files
// that match the provided glob patterns and returns them in matches.
// If a pattern contains a /, it is matched against the path relative to root
// (ignoring leading slashes). Otherwise it is matched only against the basename.
// Any non-regular files that match will be returned in skipped.
// If there is an error, matches and skipped may be incomplete or empty.
func GlobTreeRegular(root string, patterns []string) (matches []string, skipped []string, err error) {
	visit := func(path string, f os.FileInfo, err error) error {
		basename := filepath.Base(path)
		relpath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, pattern := range patterns {
			strToMatch := basename
			if strings.Contains(pattern, "/") {
				if strings.HasPrefix(pattern, "/") {
					pattern = pattern[1:]
				}
				strToMatch = relpath
			}
			if m, e := filepath.Match(pattern, strToMatch); e != nil {
				return e
			} else if m {
				if !f.Mode().IsRegular() {
					skipped = append(skipped, path)
				} else {
					matches = append(matches, path)
				}
			}
		}
		return nil
	}

	err = filepath.Walk(root, visit)
	return matches, skipped, err
}
