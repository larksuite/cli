// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package payload

import "errors"

// These causes distinguish shared payload failures without changing their
// typed envelopes. Callers can adapt recovery hints to their command's flags.
var (
	// ErrSensitiveFiles identifies credential files rejected by Guard.
	ErrSensitiveFiles = errors.New("payload contains credential files") //nolint:forbidigo // Sentinel used only as a typed validation error cause, never returned as a command failure.
	// ErrEntryConflict identifies an entry rename colliding with index.html.
	ErrEntryConflict = errors.New("publication entry conflicts with index.html") //nolint:forbidigo // Sentinel used only as a typed validation error cause, never returned as a command failure.
	// ErrDependencyLimit identifies a dependency count or depth limit.
	ErrDependencyLimit = errors.New("publication dependency limit exceeded") //nolint:forbidigo // Sentinel used only as a typed validation error cause, never returned as a command failure.
)
