// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package runtimebootstrap

import (
	"errors"
	"os"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/larksuite/cli/internal/vfs"
)

// resolveEdition keeps Standard on the ordinary Profile/provider path. It
// deliberately knows nothing about managed credential modes or protocols; its
// only external-runtime responsibility is a fail-closed presence sentinel.
func resolveEdition(_ string) *Result {
	result := &Result{Plan: runtimeplan.Default()}
	systemConfigPresent, err := inspectStandardSystemConfig()
	if err != nil {
		result.Plan = runtimeplan.Failed(err, runtimeplan.MetadataEmbeddedOnly)
		return result
	}
	if systemConfigPresent {
		result.Plan = runtimeplan.Failed(
			errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"system external credential configuration requires the lark-cli Extended edition").
				WithHint("install lark-cli Extended or ask the administrator to remove external-credential.json"),
			runtimeplan.MetadataEmbeddedOnly,
		)
		return result
	}

	// A Profile snapshot is optional for the ordinary provider chain.
	// Preserving nil on missing, malformed, or unreadable Profile data lets a
	// complete environment or compile-time provider retain its established
	// fallback behavior.
	profile, err := core.LoadMultiAppConfig()
	if err == nil {
		result.ProfileConfig = profile
	}
	return result
}

// inspectStandardSystemConfig distinguishes an absent sentinel from an
// inaccessible one. Both presence and inspection failures fail closed, but an
// inspection failure must not be misreported as an edition mismatch.
func inspectStandardSystemConfig() (bool, error) {
	path := standardSystemConfigPath()
	_, err := vfs.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, errs.NewConfigError(errs.SubtypeInvalidConfig,
		"cannot inspect system external credential configuration at %q: %v", path, err).
		WithHint("ask the system administrator to restore access to the configuration path and its parent directories").
		WithCause(err)
}

func standardSystemConfigPath() string {
	if build.Version == "DEV" {
		if path := os.Getenv(envvars.CliExternalCredentialConfig); path != "" {
			return path
		}
	}
	return defaultStandardSystemConfigPath()
}
