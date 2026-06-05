// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package hook is the internal Hook dispatch implementation. It owns:
//
//   - Registry         the in-memory data store mapping (Stage|Event) ->
//     registered hooks for fast dispatch
//   - Install(root, …) the entry point that wraps every command's RunE
//     so Before/After Observers and Wrap chains fire
//     around the command's business logic, including
//     the denial guard that physically isolates
//     pruned commands from Wrap.
//   - Emit(event, …)   the lifecycle event firing helper used by the
//     Bootstrap pipeline.
//
// Plugins NEVER import this package -- they only ever see
// extension/platform. The Registrar contract is implemented inside
// internal/platform, which delegates to this Registry after validating
// the plugin's calls (staging + atomic commit).
package hook
