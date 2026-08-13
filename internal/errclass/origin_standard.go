// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package errclass

// Standard keeps the pre-Extended API error envelope unchanged.
func larkErrorOrigin() string { return "" }
