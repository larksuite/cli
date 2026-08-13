// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package output

// Standard preserves the existing error envelope byte shape.
func setDefaultErrorOrigin(err error) error { return err }

func defaultErrorOrigin() string { return "" }
