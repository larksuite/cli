// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package localfileio

func validateLocalInputPlatform(string) error { return nil }
