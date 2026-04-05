// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"os/exec"
	"runtime"
)

// commandStarter abstracts process launching for testing.
var commandStarter = func(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// OpenBrowser attempts to open a URL in the user's default browser.
// Returns true if the attempt was made, false if the platform is unsupported
// or the command failed to start.
func OpenBrowser(url string) bool {
	return openBrowser(url, runtime.GOOS)
}

// openBrowser is the internal implementation that accepts goos for testability.
func openBrowser(url, goos string) bool {
	var cmd *exec.Cmd
	switch goos {
	case "darwin":
		cmd = commandStarter("open", url)
	case "linux":
		cmd = commandStarter("xdg-open", url)
	case "windows":
		cmd = commandStarter("cmd", "/c", "start", "", url)
	default:
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	go cmd.Wait()
	return true
}
