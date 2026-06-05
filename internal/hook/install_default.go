// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package hook

import "os"

// defaultStderr is the real os.Stderr writer. Kept in a separate file so
// tests can replace `stderr` (in install.go) with a buffer without
// shadowing this variable.
var defaultStderr = os.Stderr //nolint:forbidigo // framework-level fallback writer; hooks fire before IOStreams plumbing is available
