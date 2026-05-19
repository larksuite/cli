// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type htmlPublishCandidate struct {
	RelPath string
	AbsPath string
	Size    int64
}

func walkHTMLPublishCandidates(rootPath string) ([]htmlPublishCandidate, error) {
	stat, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", rootPath, err)
	}
	if !stat.IsDir() {
		return []htmlPublishCandidate{{
			RelPath: filepath.Base(rootPath),
			AbsPath: rootPath,
			Size:    stat.Size(),
		}}, nil
	}

	var out []htmlPublishCandidate
	err = filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		out = append(out, htmlPublishCandidate{
			RelPath: filepath.ToSlash(rel),
			AbsPath: path,
			Size:    info.Size(),
		})
		return nil
	})
	return out, err
}
