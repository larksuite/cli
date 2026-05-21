// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
)

type htmlPublishCandidate struct {
	RelPath string
	AbsPath string
	Size    int64
}

// walkHTMLPublishCandidates walks rootPath and returns each regular file as a
// candidate. Stat goes through fileio so SafeInputPath validation runs on the
// root; the directory walk itself uses filepath.WalkDir because runtime.FileIO
// has no WalkDir equivalent today.
func walkHTMLPublishCandidates(fio fileio.FileIO, rootPath string) ([]htmlPublishCandidate, error) {
	stat, err := fio.Stat(rootPath)
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
	//nolint:forbidigo // fileio has no WalkDir; rootPath is already validated above via fio.Stat -> SafeInputPath.
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
		// 只接受 regular file —— symlink / device / pipe / socket 都跳过。
		// symlink 不跟随是设计决策（避免 loop + out-of-root 引用），且 fio.Open 也会拒非 regular。
		if !info.Mode().IsRegular() {
			return nil
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
