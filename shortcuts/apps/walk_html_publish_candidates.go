// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
)

type htmlPublishCandidate struct {
	RelPath string
	AbsPath string
	Size    int64
}

// isUnsafeRelPath reports whether a forward-slash relative path contains
// anything that should never be written into a tar header or treated as
// inside-root: leading slash (absolute), .. as a path component (start /
// middle / end / whole), or an embedded null byte. Component-aware so it
// does not false-positive on legitimate filenames that contain ".." as a
// substring (e.g. "archive.tar..bak").
func isUnsafeRelPath(rel string) bool {
	return strings.HasPrefix(rel, "/") ||
		rel == ".." ||
		strings.HasPrefix(rel, "../") ||
		strings.Contains(rel, "/../") ||
		strings.HasSuffix(rel, "/..") ||
		strings.ContainsRune(rel, 0)
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
		// Skip a stray git repo: a directory named .git skips the whole subtree,
		// and a .git file (the gitdir pointer used by submodules/worktrees) is
		// skipped too.
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
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
		relSlash := filepath.ToSlash(rel)
		// Defense in depth: WalkDir + Rel inside rootPath should never yield a
		// path with .. components, but a future logic change or unusual
		// filesystem layout shouldn't be able to inject one into RelPath.
		// Mirrors the same guard at tar entry write time.
		if isUnsafeRelPath(relSlash) {
			return fmt.Errorf("walker produced unsafe relative path %q for %s", relSlash, path)
		}
		out = append(out, htmlPublishCandidate{
			RelPath: relSlash,
			AbsPath: path,
			Size:    info.Size(),
		})
		return nil
	})
	return out, err
}
