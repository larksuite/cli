// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package event

// ignoreBrokenPipe is a no-op on Windows (no SIGPIPE; closed-pipe writes return ERROR_BROKEN_PIPE directly).
func ignoreBrokenPipe() {}
