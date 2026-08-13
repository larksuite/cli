// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended && windows

package externalcredential

import (
	"errors"

	"golang.org/x/sys/windows"
)

func trustedCredentialProcessEnvironment() ([]string, error) {
	windowsDir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil, err
	}
	if windowsDir == "" {
		return nil, errors.New("system Windows directory is empty")
	}
	// Resolve the directory through the OS API rather than trusting a caller-
	// supplied SYSTEMROOT/WINDIR value.
	return []string{
		"SYSTEMROOT=" + windowsDir,
		"WINDIR=" + windowsDir,
	}, nil
}
