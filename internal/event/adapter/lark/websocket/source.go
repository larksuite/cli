// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package source is a pluggable event source abstraction (separate package to keep
// business registrations free of SDK transitive deps).
package websocket

// StatusNotifier surfaces source lifecycle states; detail is free-form
// context. A function alias so implementations structurally satisfy the
// bus-side Source port without importing it.
type StatusNotifier = func(state, detail string)
