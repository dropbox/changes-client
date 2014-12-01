// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glob

import (
	"os"
	"path/filepath"
	"strings"
)

func GlobTree(root string, patterns []string) ([]string, error) {
	matches := []string{}

	visit := func(path string, f os.FileInfo, err error) error {
		pathBits := strings.Split(path, "/")
		filename := pathBits[len(pathBits)-1]
		for _, pattern := range patterns {
			m, err := filepath.Match(pattern, filename)
			if err != nil {
				return err
			}
			if m {
				matches = append(matches, path)
			}
		}
		return nil
	}

	err := filepath.Walk(root, visit)
	return matches, err
}
