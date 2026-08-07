// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package outputdir validates and creates directories used for command output.
//
// It exists so shortcuts/common can offer EnsureOutputDir without importing
// internal/vfs: the depguard rule shortcuts-no-vfs denies vfs to every file under
// shortcuts/, shortcuts/common included, and unlike the layering rule
// shortcuts-runtime-gate it grants the runtime gate no exemption. Inlining this
// body into shortcuts/common builds and passes the layering gate, then fails lint.
package outputdir

import (
	"path/filepath"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// Ensure creates an output directory with owner-only permissions.
func Ensure(path string) error {
	if !filepath.IsAbs(path) {
		resolved, err := validate.SafeOutputPath(path)
		if err != nil {
			return err
		}
		path = resolved
	}
	return vfs.MkdirAll(path, 0700)
}
