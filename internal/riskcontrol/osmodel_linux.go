//go:build linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

// readDeviceModel returns a stable device model for Linux. DMI and device-tree
// values vary widely and can expose the host or virtualization platform when
// the CLI runs in a container or sandbox.
func readDeviceModel() string {
	return readLinuxDeviceModel()
}

func readLinuxDeviceModel() string {
	return "linux"
}
