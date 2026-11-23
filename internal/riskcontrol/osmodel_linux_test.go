//go:build linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import "testing"

func TestReadDeviceModelReturnsLinux(t *testing.T) {
	if got := readDeviceModel(); got != "linux" {
		t.Fatalf("readDeviceModel() = %q, want %q", got, "linux")
	}
}

func TestReadLinuxDeviceModel(t *testing.T) {
	if got := readLinuxDeviceModel(); got != "linux" {
		t.Fatalf("readLinuxDeviceModel() = %q, want %q", got, "linux")
	}
}
