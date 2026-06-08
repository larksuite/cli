// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"regexp"
	"strings"
	"testing"
)

func TestAppsShortcutsHaveExamples(t *testing.T) {
	realAppID := regexp.MustCompile(`app_[a-z0-9]{6,}`)
	email := regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phone := regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	for _, s := range Shortcuts() {
		hasExample := false
		for _, tip := range s.Tips {
			if strings.HasPrefix(tip, "Example: lark-cli apps +") {
				hasExample = true
			}
			if realAppID.MatchString(tip) {
				t.Errorf("%s tip leaks real-looking app id (use <app_id>): %q", s.Command, tip)
			}
			if email.MatchString(tip) || phone.MatchString(tip) {
				t.Errorf("%s tip leaks PII: %q", s.Command, tip)
			}
		}
		if !hasExample {
			t.Errorf("%s has no \"Example: lark-cli apps +...\" tip", s.Command)
		}
	}
}

func TestHighFreqCommandsHaveMultipleExamples(t *testing.T) {
	want := map[string]int{"+chat": 2, "+access-scope-set": 2}
	for _, s := range Shortcuts() {
		min, ok := want[s.Command]
		if !ok {
			continue
		}
		n := 0
		for _, tip := range s.Tips {
			if strings.HasPrefix(tip, "Example: lark-cli apps +") {
				n++
			}
		}
		if n < min {
			t.Errorf("%s has %d Example tips, want >= %d", s.Command, n, min)
		}
	}
}
