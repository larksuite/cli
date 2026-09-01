// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/vfs"
)

func resolveInstallDestinations(opts InstallOptions) (string, []string, error) {
	executable := opts.ExecutablePath
	if executable == "" {
		var err error
		executable, err = vfs.Executable()
		if err != nil {
			return "", nil, err
		}
		executable, err = vfs.EvalSymlinks(executable)
		if err != nil {
			return "", nil, err
		}
	}
	if opts.SkillsDir != "" {
		return executable, []string{opts.SkillsDir}, nil
	}
	skillsDirs, err := discoverSkillsDirs()
	if err != nil {
		return "", nil, err
	}
	return executable, skillsDirs, nil
}

// discoverSkillsDirs mirrors the destinations managed by `skills add -g`
// without invoking Node, which keeps manifest installation self-contained.
func discoverSkillsDirs() ([]string, error) {
	home, err := vfs.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dirs := []string{filepath.Join(home, ".agents", "skills")}
	dirs = appendDetectedSkillsDir(dirs, os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home, ".claude"))
	dirs = appendDetectedSkillsDir(dirs, os.Getenv("CODEX_HOME"), filepath.Join(home, ".codex"))
	return uniquePaths(dirs), nil
}

func appendDetectedSkillsDir(dirs []string, configuredRoot, defaultRoot string) []string {
	root := configuredRoot
	if root == "" {
		root = defaultRoot
		if info, err := vfs.Stat(root); err != nil || !info.IsDir() {
			return dirs
		}
	}
	return append(dirs, filepath.Join(root, "skills"))
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		result = append(result, path)
	}
	return result
}
