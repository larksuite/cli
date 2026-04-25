// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package busdiscover

import (
	"os/exec"
	"strings"
	"testing"
)

// LC_ALL=C must be forced when invoking ps so weekday/month parsing works on localized hosts.
func TestPSLocaleForcedC(t *testing.T) {
	out, err := exec.Command("locale", "-a").Output()
	if err != nil {
		t.Skipf("locale command unavailable: %v", err)
	}
	if !strings.Contains(string(out), "zh_CN") {
		t.Skip("no non-C locale (zh_CN) installed; can't prove LC_ALL override")
	}

	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_TIME", "zh_CN.UTF-8")
	t.Setenv("LANG", "zh_CN.UTF-8")

	s := newPlatformScanner().(*unixScanner)
	out, err = s.runPS()
	if err != nil {
		t.Skipf("runPS failed (likely no ps on this host): %v", err)
	}
	text := string(out)

	englishWeekdays := []string{"Mon ", "Tue ", "Wed ", "Thu ", "Fri ", "Sat ", "Sun "}
	foundEnglish := false
	for _, d := range englishWeekdays {
		if strings.Contains(text, d) {
			foundEnglish = true
			break
		}
	}
	if !foundEnglish {
		t.Errorf("ps output contains no English weekday abbreviations; LC_ALL=C not effective. sample:\n%s",
			firstNLines(text, 5))
	}
}

func firstNLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
