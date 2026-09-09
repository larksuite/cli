// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"
)

func TestResolveEntryCombinations(t *testing.T) {
	cases := []struct {
		name      string
		entryFlag string
		rootNames []string
		want      string
		wantErr   string
	}{
		{"未给且有 index.html", "", []string{"index.html", "a.css"}, "index.html", ""},
		{"未给且无 index.html", "", []string{"a.html"}, "", "no entry file"},
		{"给了且无 index.html", "page.html", []string{"page.html"}, "page.html", ""},
		{"给了且有 index.html", "page.html", []string{"index.html", "page.html"}, "", "entry conflict"},
		{"给了但不在目录下", "gone.html", []string{"other.html"}, "", "not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveEntry(tc.entryFlag, tc.rootNames)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got err %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateEntryFileName(t *testing.T) {
	bad := []string{"sub/page.html", `sub\page.html`, "notes.txt", "", "p\x00.html"}
	for _, in := range bad {
		if err := ValidateEntryFileName(in); err == nil {
			t.Errorf("ValidateEntryFileName(%q) = nil, want error", in)
		}
	}
	if err := ValidateEntryFileName("page.html"); err != nil {
		t.Errorf("ValidateEntryFileName(\"page.html\") = %v, want nil", err)
	}
}

func TestDeriveAppName(t *testing.T) {
	cases := []struct{ absEntry, want string }{
		{"/tmp/work/report.html", "report"},
		{"/tmp/work/index.html", "work"},
		{"/index.html", "html-app"},
		{"/tmp/work/INDEX.HTML", "work"},
	}
	for _, tc := range cases {
		if got := DeriveAppName(tc.absEntry); got != tc.want {
			t.Errorf("DeriveAppName(%q) = %q, want %q", tc.absEntry, got, tc.want)
		}
	}
}
