// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// TestShellPayloadHints pins both shells' prescriptions. The windows branch is
// the one that matters: PowerShell has no `<` operator, so the POSIX form is
// not merely unidiomatic there — it is a second failure, on a line this CLI
// told the caller to run.
func TestShellPayloadHints(t *testing.T) {
	t.Parallel()

	t.Run("windows never prescribes input redirection", func(t *testing.T) {
		t.Parallel()
		for _, got := range []string{
			mangledPayloadHintFor("cells", "windows"),
			outOfTreeFileHintFor("csv", "windows"),
		} {
			if strings.Contains(got, "- <") {
				t.Errorf("hint prescribes `<`, which PowerShell reserves: %q", got)
			}
			// The pipe is out too: PowerShell 5 re-encodes non-ASCII through
			// it, which is the same broken payload by another route.
			if strings.Contains(got, "Get-Content") {
				t.Errorf("hint prescribes the pipe, which re-encodes: %q", got)
			}
			if !strings.Contains(got, `"@./`) {
				t.Errorf("hint should land on a quoted @file, got %q", got)
			}
		}
		if got := payloadStdinFormFor("sheets", "windows"); got != "" {
			t.Errorf("windows has no usable stdin form, got %q", got)
		}
	})

	t.Run("windows names the cause, not just the fix", func(t *testing.T) {
		t.Parallel()
		got := mangledPayloadHintFor("cells", "windows")
		// The caller quoted the argument and has no reason to suspect it.
		for _, want := range []string{"single quotes do not prevent", "never inline JSON on this shell", "re-encodes non-ASCII"} {
			if !strings.Contains(got, want) {
				t.Errorf("hint should carry %q, got %q", want, got)
			}
		}
	})

	t.Run("posix keeps the redirection form", func(t *testing.T) {
		t.Parallel()
		got := mangledPayloadHintFor("cells", "linux")
		if !strings.Contains(got, "--cells - < payload.json") {
			t.Errorf("hint should keep the POSIX stdin form, got %q", got)
		}
	})

	t.Run("both shells lead with the universal @file form", func(t *testing.T) {
		t.Parallel()
		for _, goos := range []string{"windows", "linux", "darwin"} {
			got := mangledPayloadHintFor("styles", goos)
			if !strings.Contains(got, "--styles \"@./payload.json\"") && !strings.Contains(got, "--styles @./payload.json") {
				t.Errorf("%s hint should name the @file form, got %q", goos, got)
			}
		}
	})
}
