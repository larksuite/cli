// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

const binaryVerifyTimeout = 10 * time.Second

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

func replaceBinary(staged, target string) (func(), error) {
	// The backup supports error-path rollback, but this process cannot recover
	// from termination between the two renames. A later installer may safely
	// promote a newly staged binary while preserving the existing backup.
	backupPath := target + ".old"
	targetExists, err := pathExists(target)
	if err != nil {
		return nil, err
	}
	backupExists, err := pathExists(backupPath)
	if err != nil {
		return nil, err
	}
	if targetExists && backupExists {
		if err := vfs.Remove(backupPath); err != nil {
			return nil, fmt.Errorf("remove stale binary backup: %w", err)
		}
		backupExists = false
	}
	if targetExists {
		if err := vfs.Rename(target, backupPath); err != nil {
			return nil, err
		}
		backupExists = true
	}
	if err := vfs.Rename(staged, target); err != nil {
		if targetExists {
			_ = vfs.Rename(backupPath, target)
		}
		return nil, err
	}
	return func() {
		if backupExists {
			_ = vfs.Remove(backupPath)
		}
	}, nil
}

func pathExists(path string) (bool, error) {
	if _, err := vfs.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
