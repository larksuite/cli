// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

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

// TreeUpdate owns the backups of an applied Skills update, including its state
// file. Call Rollback if related installation work fails, or Finalize after it
// succeeds. Do not finalize a failed rollback: its backups are needed for recovery.
type TreeUpdate struct {
	targets      []*installedTree
	restoreState func() error
}

// Rollback restores target directories in reverse installation order, then the
// exact previous state file. Failed directory restores retain their backups.
func (u *TreeUpdate) Rollback() error {
	var failures []error
	for i := len(u.targets) - 1; i >= 0; i-- {
		failures = append(failures, u.targets[i].rollback())
	}
	if u.restoreState != nil {
		failures = append(failures, u.restoreState())
	}
	return errors.Join(failures...)
}

// Finalize removes staging directories and backups after a successful update.
func (u *TreeUpdate) Finalize() {
	for _, target := range u.targets {
		target.finalize()
	}
}

// SyncPreparedTree installs a complete official Skills tree and records its
// state. The returned update retains the backups until the caller rolls back
// or finalizes the surrounding installation.
func SyncPreparedTree(opts PreparedTreeOptions) (*TreeUpdate, error) {
	official, err := listPreparedSkills(opts.Root)
	if err != nil {
		return nil, err
	}
	previous, readable, err := ReadState()
	if err != nil && !errors.Is(err, ErrUnreadableState) {
		return nil, fmt.Errorf("read Skills state: %w", err)
	}
	if err != nil {
		previous, readable = nil, false
	}
	restoreState, err := SnapshotState()
	if err != nil {
		return nil, fmt.Errorf("snapshot Skills state: %w", err)
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
		return nil, err
	}
	update, err := installPreparedToTargets(opts.Root, targets, plan)
	if err != nil {
		return nil, err
	}
	update.restoreState = restoreState

	state := NewCompleteState(opts.Version, LayoutSeparate, official, previous)
	state.SourceIdentity = opts.SourceIdentity
	if err := WriteState(state); err != nil {
		cause := fmt.Errorf("write Skills state: %w", err)
		if rollbackErr := update.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("%w (%w)", cause, rollbackErr)
		}
		return nil, cause
	}
	return update, nil
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
	slices.Sort(names)
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

func installPreparedToTargets(root string, targets []string, plan SyncPlan) (*TreeUpdate, error) {
	update := &TreeUpdate{}
	for _, target := range targets {
		installed, err := installPrepared(root, target, plan)
		if err != nil {
			return nil, failPreparedAfterRollback(fmt.Errorf("install Skills to %s: %w", target, err), update.Rollback)
		}
		update.targets = append(update.targets, installed)
	}
	return update, nil
}

// installedTree records only completed moves for one target directory, so a
// failure partway through installation can undo exactly those changes.
type installedTree struct {
	target, stage, backup string
	movedOld, movedNew    []string
}

func (t *installedTree) finalize() {
	_ = vfs.RemoveAll(t.stage)
	_ = vfs.RemoveAll(t.backup)
}

func (t *installedTree) rollback() error {
	var failures []error
	for i := len(t.movedNew) - 1; i >= 0; i-- {
		failures = append(failures, vfs.RemoveAll(filepath.Join(t.target, t.movedNew[i])))
	}
	for i := len(t.movedOld) - 1; i >= 0; i-- {
		name := t.movedOld[i]
		failures = append(failures, vfs.Rename(filepath.Join(t.backup, name), filepath.Join(t.target, name)))
	}
	_ = vfs.RemoveAll(t.stage)
	if err := errors.Join(failures...); err != nil {
		return err // keep the backup for manual recovery
	}
	_ = vfs.RemoveAll(t.backup)
	return nil
}

func installPrepared(root, target string, plan SyncPlan) (*installedTree, error) {
	parent := filepath.Dir(target)
	if err := vfs.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	stage, err := vfs.MkdirTemp(parent, ".lark-cli-skills-new-*")
	if err != nil {
		return nil, err
	}
	backup, err := vfs.MkdirTemp(parent, ".lark-cli-skills-old-*")
	if err != nil {
		_ = vfs.RemoveAll(stage)
		return nil, err
	}
	installed := &installedTree{target: target, stage: stage, backup: backup}
	for _, name := range plan.ToUpdate {
		// Both paths are bounded CLI-managed host directories; the standard
		// library preserves the source tree without another copy implementation.
		if err := os.CopyFS(filepath.Join(stage, name), os.DirFS(filepath.Join(root, name))); err != nil { //nolint:forbidigo
			installed.finalize()
			return nil, err
		}
	}
	if err := vfs.MkdirAll(target, 0o755); err != nil {
		installed.finalize()
		return nil, err
	}
	for _, name := range plan.CleanupOfficial {
		current := filepath.Join(target, name)
		if _, err := vfs.Stat(current); err == nil {
			if err := vfs.Rename(current, filepath.Join(backup, name)); err != nil {
				return nil, failPreparedAfterRollback(err, installed.rollback)
			}
			installed.movedOld = append(installed.movedOld, name)
		} else if !os.IsNotExist(err) {
			return nil, failPreparedAfterRollback(err, installed.rollback)
		}
		if slices.Contains(plan.ToUpdate, name) {
			if err := vfs.Rename(filepath.Join(stage, name), current); err != nil {
				return nil, failPreparedAfterRollback(err, installed.rollback)
			}
			installed.movedNew = append(installed.movedNew, name)
		}
	}
	return installed, nil
}

func failPreparedAfterRollback(cause error, rollback func() error) error {
	if err := rollback(); err != nil {
		return fmt.Errorf("%w (rollback failed: %w; backup retained)", cause, err)
	}
	return cause
}
