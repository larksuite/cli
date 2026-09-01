// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/vfs"
)

const binaryVerifyTimeout = 10 * time.Second

// InstallOptions supplies destinations and test seams for a distribution update.
type InstallOptions struct {
	ExecutablePath string
	// SkillsDir overrides automatic Agent directory discovery when non-empty.
	SkillsDir    string
	VerifyBinary func(path, version string) error
}

// Install downloads, verifies, and commits the configured Skills and binary
// resources as one rollback-capable local transaction. The executable is
// committed last.
func Install(ctx context.Context, manifest *Manifest, opts InstallOptions) error {
	prepared, err := prepareUpdate(ctx, manifest)
	if err != nil {
		return ClassifyError("failed to prepare distribution update", err)
	}
	defer prepared.cleanup()
	if err := installPrepared(prepared, opts); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to install distribution update: %s", err).
			WithHint("Retry with `lark-cli update --force`.").
			WithCause(err)
	}
	return nil
}

func installPrepared(prepared *preparedUpdate, opts InstallOptions) error {
	if prepared == nil || prepared.Manifest == nil {
		return fmt.Errorf("prepared distribution update is required")
	}
	executable, skillsDirs, err := resolveInstallDestinations(opts)
	if err != nil {
		return err
	}
	if opts.VerifyBinary == nil {
		opts.VerifyBinary = verifyBinaryVersion
	}

	stagedBinary, err := stageBinary(prepared.BinaryPath, executable)
	if err != nil {
		return fmt.Errorf("stage binary: %w", err)
	}
	defer func() { _ = vfs.Remove(stagedBinary) }()
	if err := opts.VerifyBinary(stagedBinary, prepared.Manifest.Version); err != nil {
		return fmt.Errorf("verify staged binary: %w", err)
	}

	previous, _, err := skillscheck.ReadState()
	if err != nil {
		return fmt.Errorf("read Skills state: %w", err)
	}
	restoreState, err := skillscheck.SnapshotState()
	if err != nil {
		return fmt.Errorf("snapshot Skills state: %w", err)
	}
	rollbackSkills, finalizeSkills, err := installSkillsToTargets(prepared, skillsDirs, previous)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		var failures []string
		if err := rollbackSkills(); err != nil {
			failures = append(failures, "Skills: "+err.Error())
		}
		if err := restoreState(); err != nil {
			failures = append(failures, "state: "+err.Error())
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w (rollback failed: %s)", cause, strings.Join(failures, "; "))
		}
		return cause
	}

	added := difference(prepared.SkillNames, officialSkills(previous))
	state := skillscheck.SkillsState{
		Version:             prepared.Manifest.Version,
		Layout:              skillscheck.LayoutSeparate,
		OfficialSkills:      prepared.SkillNames,
		UpdatedSkills:       prepared.SkillNames,
		AddedOfficialSkills: added,
		UpdatedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	if err := skillscheck.WriteState(state); err != nil {
		return rollback(fmt.Errorf("write Skills state: %w", err))
	}

	finalizeBinary, err := replaceBinary(stagedBinary, executable)
	if err != nil {
		return rollback(fmt.Errorf("replace binary: %w", err))
	}
	finalizeSkills()
	finalizeBinary()
	return nil
}

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

func stageBinary(source, executable string) (string, error) {
	if err := vfs.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		return "", err
	}
	in, err := vfs.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := vfs.CreateTemp(filepath.Dir(executable), ".lark-cli-new-*")
	if err != nil {
		return "", err
	}
	path := out.Name()
	keep := false
	defer func() {
		_ = out.Close()
		if !keep {
			_ = vfs.Remove(path)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	if err := out.Chmod(0o755); err != nil {
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}

func verifyBinaryVersion(path, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), binaryVerifyTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput() //nolint:gosec // path is the checksum-verified staged binary.
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("binary verification timed out after %s", binaryVerifyTimeout)
	}
	if err != nil {
		return fmt.Errorf("run --version: %w", err)
	}
	if !matchesVersionOutput(string(output), version) {
		return fmt.Errorf("binary reported %q, want version %q", strings.TrimSpace(string(output)), version)
	}
	return nil
}

func matchesVersionOutput(output, version string) bool {
	return strings.TrimSpace(output) == "lark-cli version "+version
}

func installSkills(prepared *preparedUpdate, target string, previous *skillscheck.SkillsState) (func() error, func(), error) {
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
	for _, name := range prepared.SkillNames {
		if err := copyTree(filepath.Join(prepared.SkillsRoot, name), filepath.Join(stage, name)); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if err := vfs.MkdirAll(target, 0o755); err != nil {
		cleanup()
		return nil, nil, err
	}
	managed := union(prepared.SkillNames, officialSkills(previous))
	movedOld := []string{}
	movedNew := []string{}
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
		return first
	}
	for _, name := range managed {
		current := filepath.Join(target, name)
		if _, err := vfs.Stat(current); err == nil {
			if err := vfs.Rename(current, filepath.Join(backup, name)); err != nil {
				_ = rollback()
				cleanup()
				return nil, nil, err
			}
			movedOld = append(movedOld, name)
		} else if !os.IsNotExist(err) {
			_ = rollback()
			cleanup()
			return nil, nil, err
		}
		if contains(prepared.SkillNames, name) {
			if err := vfs.Rename(filepath.Join(stage, name), current); err != nil {
				_ = rollback()
				cleanup()
				return nil, nil, err
			}
			movedNew = append(movedNew, name)
		}
	}
	return rollback, func() { _ = vfs.RemoveAll(stage); _ = vfs.RemoveAll(backup) }, nil
}

func installSkillsToTargets(prepared *preparedUpdate, targets []string, previous *skillscheck.SkillsState) (func() error, func(), error) {
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
	finalizeAll := func() {
		for _, finalize := range finalizers {
			finalize()
		}
	}
	for _, target := range targets {
		rollback, finalize, err := installSkills(prepared, target, previous)
		if err != nil {
			_ = rollbackAll()
			finalizeAll()
			return nil, nil, fmt.Errorf("install Skills to %s: %w", target, err)
		}
		rollbacks = append(rollbacks, rollback)
		finalizers = append(finalizers, finalize)
	}
	return rollbackAll, finalizeAll, nil
}

func copyTree(source, destination string) error {
	entries, err := vfs.ReadDir(source)
	if err != nil {
		return err
	}
	if err := vfs.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		src, dst := filepath.Join(source, name), filepath.Join(destination, name)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if err := copyTree(src, dst); err != nil {
				return err
			}
			continue
		}
		in, err := vfs.Open(src)
		if err != nil {
			return err
		}
		perm := os.FileMode(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			perm = 0o755
		}
		out, err := vfs.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeOutErr := out.Close()
		closeInErr := in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutErr != nil {
			return closeOutErr
		}
		if closeInErr != nil {
			return closeInErr
		}
	}
	return nil
}

func replaceBinary(staged, target string) (func(), error) {
	backupPath := target + ".old"
	if _, err := vfs.Stat(backupPath); err == nil {
		if err := vfs.Remove(backupPath); err != nil {
			return nil, fmt.Errorf("remove stale binary backup: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := vfs.Rename(target, backupPath); err != nil {
		return nil, err
	}
	if err := vfs.Rename(staged, target); err != nil {
		_ = vfs.Rename(backupPath, target)
		return nil, err
	}
	return func() { _ = vfs.Remove(backupPath) }, nil
}

func officialSkills(state *skillscheck.SkillsState) []string {
	if state == nil || state.OfficialSkillsUnknown {
		return nil
	}
	return state.OfficialSkills
}

func union(a, b []string) []string {
	set := map[string]bool{}
	for _, values := range [][]string{a, b} {
		for _, v := range values {
			set[v] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func difference(a, b []string) []string {
	result := []string{}
	for _, value := range a {
		if !contains(b, value) {
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
