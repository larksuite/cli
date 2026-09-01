// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/larksuite/cli/internal/vfs"
)

// PreparedTreeOptions describes a complete, already-extracted official Skills
// tree. TargetDir overrides automatic agent-directory discovery when non-empty.
type PreparedTreeOptions struct {
	Root           string
	Version        string
	SourceIdentity string
	TargetDir      string
}

// SyncPreparedTree installs a complete official Skills tree and records its
// state. The returned rollback is kept by callers until related update work is
// committed; finalize removes temporary backups after a successful commit.
func SyncPreparedTree(opts PreparedTreeOptions) (rollback func() error, finalize func(), err error) {
	official, err := listPreparedSkills(opts.Root)
	if err != nil {
		return nil, nil, err
	}
	previous, readable, err := ReadState()
	if err != nil {
		return nil, nil, fmt.Errorf("read Skills state: %w", err)
	}
	restoreState, err := SnapshotState()
	if err != nil {
		return nil, nil, fmt.Errorf("snapshot Skills state: %w", err)
	}
	plan := PlanSync(SyncInput{
		Version:        opts.Version,
		OfficialSkills: official,
		PreviousState:  previous,
		StateReadable:  readable,
		Force:          true,
	})
	targets, err := preparedSkillsTargets(opts.TargetDir)
	if err != nil {
		return nil, nil, err
	}
	rollbackFiles, finalizeFiles, err := installPreparedToTargets(opts.Root, targets, plan)
	if err != nil {
		return nil, nil, err
	}
	rollbackAll := func() error {
		return errors.Join(rollbackFiles(), restoreState())
	}

	state := NewCompleteState(opts.Version, LayoutSeparate, official, previous)
	state.SourceIdentity = opts.SourceIdentity
	if err := WriteState(state); err != nil {
		cause := fmt.Errorf("write Skills state: %w", err)
		if rollbackErr := rollbackAll(); rollbackErr != nil {
			return nil, nil, fmt.Errorf("%w (%w)", cause, rollbackErr)
		}
		return nil, nil, cause
	}
	return rollbackAll, finalizeFiles, nil
}

func listPreparedSkills(root string) ([]string, error) {
	entries, err := vfs.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("skills artifact contains no Skills")
	}
	sort.Strings(names)
	return names, nil
}

func preparedSkillsTargets(override string) ([]string, error) {
	if override != "" {
		return []string{override}, nil
	}
	home, err := vfs.UserHomeDir()
	if err != nil {
		return nil, err
	}
	targets := []string{filepath.Join(home, ".agents", "skills")}
	targets = appendDetectedTarget(targets, os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home, ".claude"))
	targets = appendDetectedTarget(targets, os.Getenv("CODEX_HOME"), filepath.Join(home, ".codex"))
	return uniquePaths(targets), nil
}

func appendDetectedTarget(targets []string, configuredRoot, defaultRoot string) []string {
	root := configuredRoot
	if root == "" {
		root = defaultRoot
		if info, err := vfs.Stat(root); err != nil || !info.IsDir() {
			return targets
		}
	}
	return append(targets, filepath.Join(root, "skills"))
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	return result
}

func installPreparedToTargets(root string, targets []string, plan SyncPlan) (func() error, func(), error) {
	rollbacks := make([]func() error, 0, len(targets))
	finalizers := make([]func(), 0, len(targets))
	rollbackAll := func() error {
		var first error
		for i := len(rollbacks) - 1; i >= 0; i-- {
			if err := rollbacks[i](); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
	for _, target := range targets {
		rollback, finalize, err := installPrepared(root, target, plan)
		if err != nil {
			return nil, nil, failPreparedAfterRollback(fmt.Errorf("install Skills to %s: %w", target, err), rollbackAll)
		}
		rollbacks = append(rollbacks, rollback)
		finalizers = append(finalizers, finalize)
	}
	return rollbackAll, func() {
		for _, finalize := range finalizers {
			finalize()
		}
	}, nil
}

func installPrepared(root, target string, plan SyncPlan) (func() error, func(), error) {
	parent := filepath.Dir(target)
	if err := vfs.MkdirAll(parent, 0o755); err != nil {
		return nil, nil, err
	}
	stage, err := vfs.MkdirTemp(parent, ".lark-cli-skills-new-*")
	if err != nil {
		return nil, nil, err
	}
	backup, err := vfs.MkdirTemp(parent, ".lark-cli-skills-old-*")
	if err != nil {
		_ = vfs.RemoveAll(stage)
		return nil, nil, err
	}
	cleanup := func() { _ = vfs.RemoveAll(stage); _ = vfs.RemoveAll(backup) }
	for _, name := range plan.ToUpdate {
		// Both paths are bounded CLI-managed host directories; the standard
		// library preserves the source tree without another copy implementation.
		if err := os.CopyFS(filepath.Join(stage, name), os.DirFS(filepath.Join(root, name))); err != nil { //nolint:forbidigo
			cleanup()
			return nil, nil, err
		}
	}
	if err := vfs.MkdirAll(target, 0o755); err != nil {
		cleanup()
		return nil, nil, err
	}
	movedOld, movedNew := []string{}, []string{}
	rollback := func() error {
		var first error
		for i := len(movedNew) - 1; i >= 0; i-- {
			if err := vfs.RemoveAll(filepath.Join(target, movedNew[i])); err != nil && first == nil {
				first = err
			}
		}
		for i := len(movedOld) - 1; i >= 0; i-- {
			name := movedOld[i]
			if err := vfs.Rename(filepath.Join(backup, name), filepath.Join(target, name)); err != nil && first == nil {
				first = err
			}
		}
		_ = vfs.RemoveAll(stage)
		if first == nil {
			_ = vfs.RemoveAll(backup)
		}
		return first
	}
	for _, name := range plan.CleanupOfficial {
		current := filepath.Join(target, name)
		if _, err := vfs.Stat(current); err == nil {
			if err := vfs.Rename(current, filepath.Join(backup, name)); err != nil {
				return nil, nil, failPreparedAfterRollback(err, rollback)
			}
			movedOld = append(movedOld, name)
		} else if !os.IsNotExist(err) {
			return nil, nil, failPreparedAfterRollback(err, rollback)
		}
		if slices.Contains(plan.ToUpdate, name) {
			if err := vfs.Rename(filepath.Join(stage, name), current); err != nil {
				return nil, nil, failPreparedAfterRollback(err, rollback)
			}
			movedNew = append(movedNew, name)
		}
	}
	return rollback, cleanup, nil
}

func failPreparedAfterRollback(cause error, rollback func() error) error {
	if err := rollback(); err != nil {
		return fmt.Errorf("%w (rollback failed: %w; backup retained)", cause, err)
	}
	return cause
}
