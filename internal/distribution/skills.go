// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/vfs"
)

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
	managed := union(prepared.SkillNames, skillscheck.KnownOfficialSkills(previous))
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
	return rollback, cleanup, nil
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

func union(a, b []string) []string {
	set := map[string]bool{}
	for _, values := range [][]string{a, b} {
		for _, value := range values {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
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
