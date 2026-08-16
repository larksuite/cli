// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import (
	"os"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/envvars"
)

func TestEnabledExactMatch(t *testing.T) {
	cases := map[string]bool{"1": true, "true": false, "on": false, "": false, " 1": false, "01": false}
	for value, want := range cases {
		t.Setenv(envvars.CliCitation, value)
		if got := Enabled(); got != want {
			t.Errorf("Enabled() with %q = %v, want %v", value, got, want)
		}
	}
}

func TestEnabledUnset(t *testing.T) {
	// t.Setenv 后手动删除，模拟未设置
	t.Setenv(envvars.CliCitation, "1")
	if err := unsetenvForTest(t); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Error("Enabled() with unset env = true, want false")
	}
}

func TestNormalize(t *testing.T) {
	in := []Citation{
		{SourceType: SourceWiki, URL: "https://a.example/1", Title: "keep-1"},
		{SourceType: SourceWiki, URL: "", Title: "drop-no-url"},
		{SourceType: SourceWiki, URL: "https://a.example/2", Title: "keep-2"},
	}
	got := Normalize(in)
	if len(got) != 2 || got[0].Title != "keep-1" || got[1].Title != "keep-2" {
		t.Fatalf("Normalize() = %#v, want 2 kept entries in order", got)
	}
	if Normalize(nil) != nil {
		t.Error("Normalize(nil) != nil")
	}
	if Normalize([]Citation{{URL: ""}}) != nil {
		t.Error("Normalize(all-dropped) != nil")
	}
}

func TestTime(t *testing.T) {
	local := time.Unix(1721996760, 0)
	wantSec := local.Format(time.RFC3339)
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"unix seconds int64", int64(1721996760), wantSec},
		{"unix seconds int", int(1721996760), wantSec},
		{"unix seconds float64", float64(1721996760), wantSec},
		{"unix seconds string", "1721996760", wantSec},
		{"unix millis string", "1721996760000", wantSec},
		{"unix millis int64", int64(1721996760000), wantSec},
		{"rfc3339 passthrough", "2026-07-27T21:26:00+08:00", "2026-07-27T21:26:00+08:00"},
		{"11-digit garbage", "17219967600", ""},
		{"non-time string", "not-a-time", ""},
		{"nil", nil, ""},
		{"negative", int64(-5), ""},
		{"zero int", int(0), ""},
		{"zero string", "0", ""},
		{"empty string", "", ""},
		{"whitespace", "   ", ""},
	}
	for _, tc := range cases {
		if got := Time(tc.in); got != tc.want {
			t.Errorf("Time(%s: %v) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
	// 本地时间串（无时区偏移）按本地时区补偏移
	localStr := "2026-07-27 21:26:00"
	parsed, _ := time.ParseInLocation("2006-01-02 15:04:05", localStr, time.Local)
	if got := Time(localStr); got != parsed.Format(time.RFC3339) {
		t.Errorf("Time(local string) = %q, want %q", got, parsed.Format(time.RFC3339))
	}
	localMin := "2026-07-27 21:26"
	parsedMin, _ := time.ParseInLocation("2006-01-02 15:04", localMin, time.Local)
	if got := Time(localMin); got != parsedMin.Format(time.RFC3339) {
		t.Errorf("Time(local minute string) = %q, want %q", got, parsedMin.Format(time.RFC3339))
	}
}

func TestIsAllocated(t *testing.T) {
	if IsAllocated(SourceUnset) {
		t.Error("IsAllocated(SourceUnset) = true")
	}
	for _, st := range []SourceType{SourceWiki, SourceDoc, SourceMessage, SourceMinute, SourceBitable, SourceSheet, SourceMeeting, SourceMeetingNote} {
		if !IsAllocated(st) {
			t.Errorf("IsAllocated(%d) = false", st)
		}
	}
	if IsAllocated(SourceType(4)) || IsAllocated(SourceType(99)) {
		t.Error("IsAllocated(unallocated int) = true")
	}
}

func unsetenvForTest(t *testing.T) error {
	t.Helper()
	return os.Unsetenv(envvars.CliCitation) // t.Setenv 已登记恢复，直接删除是安全的
}
