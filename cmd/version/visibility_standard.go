// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package version

// Standard keeps its historical help surface unchanged. The command remains
// directly callable for release identity verification.
func hideVersionCommand() bool { return true }
