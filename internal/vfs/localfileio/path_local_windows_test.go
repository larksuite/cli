// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import "testing"

func TestLocalInputPath_RejectsWindowsNetworkDeviceAndReservedPaths(t *testing.T) {
	for _, input := range []string{
		`\\server\share\report.pdf`,
		`//server/share/report.pdf`,
		`\\.\pipe\upload`,
		`\\?\C:\Users\agent\report.pdf`,
		`\\?\UNC\server\share\report.pdf`,
		`\??\C:\Users\agent\report.pdf`,
		`C:\Users\agent\NUL.txt`,
		`CON`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := LocalInputPath(input); err == nil {
				t.Fatalf("LocalInputPath(%q) unexpectedly succeeded", input)
			}
		})
	}
}
