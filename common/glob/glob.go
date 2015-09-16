// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glob

import (
	"os"
	"path/filepath"
)

// GlobTreeRegular walks root looking for regular (non-dir, non-device) files
// that match the provided glob patterns and returns them in matches.
// Any non-regular files that match will be returned in skipped.
// If there is an error, matches and skipped may be incomplete or empty.
func GlobTreeRegular(root string, patterns []string) (matches []string, skipped []string, err error) {
	visit := func(path string, f os.FileInfo, err error) error {
		filename := filepath.Base(path)
		for _, pattern := range patterns {
			if m, e := filepath.Match(pattern, filename); e != nil {
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
