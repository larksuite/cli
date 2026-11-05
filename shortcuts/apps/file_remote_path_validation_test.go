// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// msysRewrittenPaths are remote paths as they reach lark-cli after Git Bash (MSYS)
// on Windows rewrote them: an argument that starts with "/" is turned into a
// Windows path under the MSYS install root before a native program receives it.
// The first two were seen on the wire; the rest cover the other mappings (drive
// letter case, /tmp, backslashes).
var msysRewrittenPaths = []string{
	"C:/Program Files/Git/1858537546760216.png",
	"I:/1软件/WorkBuddy/resources/vendor/PortableGit/1880731481282624.jpg",
	"d:/Git/1858537546760216.png",
	"C:/Users/me/AppData/Local/Temp/1858537546760216.png",
	`C:\Users\me\1858537546760216.png`,
}

// TestRemoteFilePathCommands_RejectMSYSRewrittenPath pins the local check on every
// command that addresses app storage by remote path. Storage answers a rewritten
// path with "file not found or no access", which reads as a missing file; only
// the client can tell that the shell changed the argument, and say how to stop it.
func TestRemoteFilePathCommands_RejectMSYSRewrittenPath(t *testing.T) {
	commands := []struct {
		name string
		args func(path string) []string
	}{
		{"+file-get", func(p string) []string {
			return []string{"+file-get", "--app-id", "app_x", "--path", p, "--as", "user"}
		}},
		{"+file-sign", func(p string) []string {
			return []string{"+file-sign", "--app-id", "app_x", "--path", p, "--as", "user"}
		}},
		{"+file-download", func(p string) []string {
			return []string{"+file-download", "--app-id", "app_x", "--path", p, "--output", "out.png", "--as", "user"}
		}},
		{"+file-delete", func(p string) []string {
			return []string{"+file-delete", "--app-id", "app_x", "--path", "/1858537546760999.pdf", "--path", p, "--yes", "--as", "user"}
		}},
		{"+file-list", func(p string) []string {
			return []string{"+file-list", "--app-id", "app_x", "--path", p, "--as", "user"}
		}},
	}
	byCommand := map[string]bool{}
	for _, sc := range Shortcuts() {
		byCommand[sc.Command] = true
	}
	for _, c := range commands {
		if !byCommand[c.name] {
			t.Fatalf("%s is no longer registered; update this test", c.name)
		}
		sc := shortcutByCommand(t, c.name)
		for _, path := range msysRewrittenPaths {
			t.Run(c.name+" "+path, func(t *testing.T) {
				chdirTemp(t)
				factory, stdout, reg := newAppsExecuteFactory(t)
				probe := sparkRequestProbe(reg)

				err := runAppsShortcut(t, sc, c.args(path), factory, stdout)
				p := requireAppsValidationProblem(t, err)
				if got := validationParamOf(t, err); got != "--path" {
					t.Errorf("param = %q, want --path", got)
				}
				if !strings.Contains(p.Message, "local Windows path") {
					t.Errorf("message = %q, want it to say the value is a local Windows path", p.Message)
				}
				for _, want := range []string{"Git Bash", "MSYS_NO_PATHCONV=1", "//1858537546760216.png"} {
					if !strings.Contains(p.Hint, want) {
						t.Errorf("hint = %q, want it to mention %q", p.Hint, want)
					}
				}
				if n := len(probe.CapturedBodies); n != 0 {
					t.Errorf("sent %d request(s) with a rewritten local path", n)
				}
			})
		}
	}
}

// TestRequireFilePath_AcceptsRemotePaths guards the other direction: storage keys
// with or without a leading slash, nested or not, still pass.
func TestRequireFilePath_AcceptsRemotePaths(t *testing.T) {
	for _, raw := range []string{
		"/1858537546760216.png",
		"  /1858537546760216.png  ",
		"/backend-state-archives/import/e3c8bf54/c7f97df2",
		"chunks/db45727c-a2ea-482a-bcab-0e237a23eab2/part_0",
		"/a:b.png",
	} {
		got, err := requireFilePath(raw)
		if err != nil {
			t.Fatalf("requireFilePath(%q) = %v, want it accepted", raw, err)
		}
		if got != strings.TrimSpace(raw) {
			t.Errorf("requireFilePath(%q) = %q, want the trimmed path", raw, got)
		}
	}
}

func shortcutByCommand(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, sc := range Shortcuts() {
		if sc.Command == command {
			return sc
		}
	}
	t.Fatalf("%s is not registered", command)
	return common.Shortcut{}
}
