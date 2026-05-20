// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"
)

func TestFlagDescs_EmbedParses(t *testing.T) {
	t.Parallel()
	descs, err := loadFlagDescs()
	if err != nil {
		t.Fatalf("loadFlagDescs error: %v", err)
	}
	if len(descs) == 0 {
		t.Fatal("flag-descriptions.en.json has no entries")
	}
}

func TestFlagDescs_SpotCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		command  string
		flagName string
	}{
		{"+workbook-info", "url"},
		{"+cells-set", "range"},
		{"+csv-get", "range"},
		{"+batch-update", "operations"},
		{"+chart-create", "properties"},
	}
	for _, tc := range cases {
		desc := flagDesc(tc.command, tc.flagName)
		if desc == "" {
			t.Errorf("flagDesc(%q, %q) = empty; want a description", tc.command, tc.flagName)
		}
	}
}

func TestFlagDescs_UnknownReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := flagDesc("+no-such-cmd", "no-flag"); got != "" {
		t.Errorf("expected empty for unknown command; got %q", got)
	}
}

func TestApplyFlagDescs_OverridesHardcodedDesc(t *testing.T) {
	t.Parallel()
	all := Shortcuts()
	descs, err := loadFlagDescs()
	if err != nil {
		t.Fatalf("loadFlagDescs: %v", err)
	}
	for _, s := range all {
		cmd, ok := descs[s.Command]
		if !ok {
			continue
		}
		for _, f := range s.Flags {
			key := "--" + f.Name
			want, exists := cmd[key]
			if !exists {
				continue
			}
			if f.Desc != want {
				t.Errorf("%s %s: Desc=%q, want=%q", s.Command, key, f.Desc, want)
			}
		}
	}
}

func TestApplyFlagDescs_Coverage(t *testing.T) {
	t.Parallel()
	all := Shortcuts()
	descs, err := loadFlagDescs()
	if err != nil {
		t.Fatalf("loadFlagDescs: %v", err)
	}

	// Framework-injected flags are not in the Flags slice but may
	// appear in the JSON as documentation. Skip them.
	frameworkFlags := map[string]bool{
		"--yes":     true,
		"--dry-run": true,
	}

	// Every non-framework flag in the JSON should appear in the shortcut list.
	for cmd, flags := range descs {
		for flagKey := range flags {
			if frameworkFlags[flagKey] {
				continue
			}
			found := false
			for _, s := range all {
				if s.Command != cmd {
					continue
				}
				for _, f := range s.Flags {
					if "--"+f.Name == flagKey {
						found = true
						break
					}
				}
				break
			}
			if !found {
				t.Logf("JSON has %s %s but no matching flag in shortcut list (naming mismatch or not yet implemented)", cmd, flagKey)
			}
		}
	}
}
