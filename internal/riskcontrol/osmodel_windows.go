//go:build windows

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import "golang.org/x/sys/windows/registry"

// systemInfoRegistryPaths lists registry locations in device-model lookup order.
var systemInfoRegistryPaths = [...]string{
	`HARDWARE\DESCRIPTION\System\BIOS`,
	`SYSTEM\CurrentControlSet\Control\SystemInformation`,
	`SYSTEM\HardwareConfig\Current`,
}

// readDeviceModel returns the first product name found in the Windows registry.
func readDeviceModel() string {
	return readWindowsDeviceModel(readWindowsRegistryModel)
}

func readWindowsRegistryModel(path string) (string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ)
	if err != nil {
		return "", err
	}
	defer key.Close()

	model, _, err := key.GetStringValue("SystemProductName")
	return model, err
}

func readWindowsDeviceModel(readRegistryModel func(string) (string, error)) string {
	for _, path := range systemInfoRegistryPaths {
		model, err := readRegistryModel(path)
		if err != nil {
			continue
		}
		if model = normalizeDeviceModel(model); model != "" {
			return model
		}
	}
	return ""
}
