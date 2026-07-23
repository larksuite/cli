//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import "golang.org/x/sys/unix"

// readDeviceModel reads the current product key first and falls back to the
// legacy model key. Trying both keys is more robust than branching on a macOS
// version because virtualized or restricted environments may expose only one.
func readDeviceModel() string {
	return readDarwinDeviceModel(unix.Sysctl)
}

func readDarwinDeviceModel(readSysctl func(string) (string, error)) string {
	for _, key := range [...]string{"hw.product", "hw.model"} {
		model, err := readSysctl(key)
		if err == nil {
			if model = normalizeDeviceModel(model); model != "" {
				return model
			}
		}
	}
	return ""
}
