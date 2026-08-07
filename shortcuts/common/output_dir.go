// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "github.com/larksuite/cli/internal/outputdir"

// EnsureOutputDir creates an output directory with owner-only permissions.
// Relative paths are validated and resolved within the working directory.
// Absolute paths are accepted for callers that already resolved them through
// SafeOutputPath or RuntimeContext.ResolveSavePath.
//
// The body stays in internal/outputdir rather than here: creating the directory
// needs internal/vfs, and the depguard rule shortcuts-no-vfs denies vfs to every
// file under shortcuts/ with no exemption for this package — the layering rule
// shortcuts-runtime-gate exempts it, the lint rule does not. So this forwarder is
// load-bearing, not a leftover hop, and inlining it fails CI while building and
// passing the layering gate.
func EnsureOutputDir(path string) error {
	return outputdir.Ensure(path)
}
