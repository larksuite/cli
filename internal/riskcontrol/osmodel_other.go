//go:build !darwin && !windows && !linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

// readDeviceModel returns an empty model on unsupported platforms.
func readDeviceModel() string {
	return ""
}
