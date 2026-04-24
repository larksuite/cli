// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package busdiscover

import (
	"os/exec"
	"strings"
	"testing"
)

// TestPSLocaleForcedC — regression for the Bug 7 finding (CodeRabbit PR
// #615). `ps -o lstart` emits weekday/month names per the caller's
// LC_TIME; on zh_CN / de_DE / fr_FR the output does NOT match the
// English "Mon Jan _2 15:04:05 2006" layout and silently drops every
// row in parseOneUnixPSLine, so orphan bus detection fails entirely
// on localized hosts.
//
// The fix forces C locale when invoking ps. This test proves it by:
// setting our own process LC_TIME to a locale that would produce
// non-English output if inherited, then calling the production runPS
// and asserting the output is English-formatted.
//
// Skipped if the `ps` command isn't available or the machine lacks
// a usable LC_TIME locale to test against.
func TestPSLocaleForcedC(t *testing.T) {
	// This test depends on a non-C locale being installed on the host —
	// otherwise setting LC_ALL=zh_CN.UTF-8 has no effect (ps falls back
	// to C) and the test passes vacuously even pre-fix. macOS bundles
	// ICU locales universally; Linux CI runners typically have only C
	// + en_US. Skip explicitly rather than silently pass.
	out, err := exec.Command("locale", "-a").Output()
	if err != nil {
		t.Skipf("locale command unavailable: %v", err)
	}
	if !strings.Contains(string(out), "zh_CN") {
		t.Skip("no non-C locale (zh_CN) installed; can't prove LC_ALL override")
	}

	// Force a non-English LC_TIME on the test process. If the production
	// scanner inherits the environment without overriding LC_ALL, the
	// child `ps` will emit localized weekday/month (e.g. "周日 4月").
	// With the fix, LC_ALL=C on cmd.Env overrides this and the output
	// stays English.
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_TIME", "zh_CN.UTF-8")
	t.Setenv("LANG", "zh_CN.UTF-8")

	s := newPlatformScanner().(*unixScanner)
	out, err = s.runPS()
	if err != nil {
		t.Skipf("runPS failed (likely no ps on this host): %v", err)
	}
	text := string(out)

	// Look for at least one English weekday abbreviation in the output.
	// Any real ps output on a multi-process system will have many lines
	// with Mon/Tue/Wed/Thu/Fri/Sat/Sun.
	englishWeekdays := []string{"Mon ", "Tue ", "Wed ", "Thu ", "Fri ", "Sat ", "Sun "}
	foundEnglish := false
	for _, d := range englishWeekdays {
		if strings.Contains(text, d) {
			foundEnglish = true
			break
		}
	}
	if !foundEnglish {
		t.Errorf("ps output contains no English weekday abbreviations; sample:\n%s\n"+
			"This suggests LC_ALL=C is NOT being set on the ps invocation — "+
			"orphan bus detection will fail on localized (zh_CN / de_DE / fr_FR) hosts.", firstNLines(text, 5))
	}
}

func firstNLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
