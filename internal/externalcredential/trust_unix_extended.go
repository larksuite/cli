// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended && !windows

package externalcredential

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

func validateAdminControlledPath(target string, executable bool) error {
	// The DEV-only path override is also a trust override: for both the system
	// configuration and its helper executable it skips root ownership and
	// group/other-writability checks on the target and all ancestors. Regular
	// file, symlink, executable, and helper SHA checks still run, but the SHA
	// comes from the same developer-controlled configuration and is not an
	// independent trust anchor. Release builds can never enable this path.
	devTrustOverride := build.Version == "DEV" && os.Getenv(envvars.CliExternalCredentialConfig) != ""
	return validateAdminControlledPathWith(target, executable, devTrustOverride, vfs.Lstat)
}

func validateAdminControlledPathWith(
	target string,
	executable bool,
	devOverride bool,
	lstat func(string) (os.FileInfo, error),
) error {
	target = filepath.Clean(target)
	current := target
	for {
		info, err := lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link", current)
		}
		if current == target {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%s is not a regular file", current)
			}
			if executable && info.Mode().Perm()&0o111 == 0 {
				return fmt.Errorf("%s is not executable", current)
			}
			if devOverride {
				break
			}
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return fmt.Errorf("%s is not owned by root", current)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s is writable by group or other users", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}
