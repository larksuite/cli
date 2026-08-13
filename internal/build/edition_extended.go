// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package build

const Edition = "extended"

func Capabilities() []string {
	return []string{"external-credential-platform"}
}
