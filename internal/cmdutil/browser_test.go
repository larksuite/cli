// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"os/exec"
	"testing"
)

func TestOpenBrowser_CommandPerPlatform(t *testing.T) {
	tests := []struct {
		goos    string
		wantCmd string
		wantOK  bool
	}{
		{"darwin", "open", true},
		{"linux", "xdg-open", true},
		{"windows", "cmd", true},
		{"freebsd", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			var gotName string
			var gotArgs []string

			orig := commandStarter
			commandStarter = func(name string, args ...string) *exec.Cmd {
				gotName = name
				gotArgs = args
				// Return a harmless command so Start() succeeds.
				return exec.Command("true")
			}
			t.Cleanup(func() { commandStarter = orig })

			ok := openBrowser("https://example.com", tt.goos)
			if ok != tt.wantOK {
				t.Fatalf("openBrowser(_, %q) = %v, want %v", tt.goos, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if gotName != tt.wantCmd {
				t.Errorf("command = %q, want %q", gotName, tt.wantCmd)
			}
			// URL must appear in args.
			found := false
			for _, a := range gotArgs {
				if a == "https://example.com" {
					found = true
				}
			}
			if !found {
				t.Errorf("URL not found in args: %v", gotArgs)
			}
		})
	}
}

func TestOpenBrowser_StartFailure(t *testing.T) {
	orig := commandStarter
	commandStarter = func(name string, args ...string) *exec.Cmd {
		return exec.Command("__nonexistent_binary__")
	}
	t.Cleanup(func() { commandStarter = orig })

	if openBrowser("https://example.com", "darwin") {
		t.Error("expected false when command fails to start")
	}
}
