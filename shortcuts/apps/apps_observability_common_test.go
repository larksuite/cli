// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

func requireAppsValidationParam(t *testing.T, err error, want string) *errs.Problem {
	t.Helper()
	p := requireAppsValidationProblem(t, err)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error with param %q, got %T: %v", want, err, err)
	}
	if validationErr.Param != want {
		t.Fatalf("param = %q, want %s", validationErr.Param, want)
	}
	return p
}

func TestAppsObservabilityValidateEnvOnlyOnline(t *testing.T) {
	if err := validateObservabilityEnv(""); err != nil {
		t.Fatalf("empty env should default/pass as online: %v", err)
	}
	if err := validateObservabilityEnv("online"); err != nil {
		t.Fatalf("online should pass: %v", err)
	}
	err := validateObservabilityEnv("dev")
	p := requireAppsValidationParam(t, err, "--environment")
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v, want invalid_argument param --environment", p)
	}
	if !strings.Contains(p.Hint, "only online is supported") {
		t.Fatalf("hint = %q, want only-online guidance", p.Hint)
	}
}

func TestAppsObservabilityPageSizeRange(t *testing.T) {
	for _, n := range []int{1, 50, 100} {
		if err := validateAppsPageSize(n); err != nil {
			t.Fatalf("page size %d should pass: %v", n, err)
		}
	}
	for _, n := range []int{0, 101} {
		err := validateAppsPageSize(n)
		requireAppsValidationParam(t, err, "--page-size")
	}
}

func TestAppsObservabilityCommonHelpers(t *testing.T) {
	if got := appScopedPath("app/x", "observability/logs"); got != "/open-apis/spark/v1/apps/app%2Fx/observability/logs" {
		t.Fatalf("appScopedPath = %q", got)
	}
	for _, env := range []string{"dev", "online"} {
		if err := validateEnvVarEnv(env); err != nil {
			t.Fatalf("validateEnvVarEnv(%q) err=%v", env, err)
		}
	}
	requireAppsValidationParam(t, validateEnvVarEnv(""), "--environment")
	requireAppsValidationParam(t, validateEnvVarEnv("boe"), "--environment")
	got := cleanRepeatedStrings([]string{" a ", "b", "a", "", "b", "c"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("cleanRepeatedStrings len=%d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cleanRepeatedStrings[%d]=%q, want %q", i, got[i], want[i])
		}
	}
	ts := time.Date(2026, 6, 23, 10, 11, 12, 123456789, time.UTC)
	if got := nsNumber(ts); got != "1782209472123456789" {
		t.Fatalf("nsNumber = %q", got)
	}
	if got := secNumber(ts); got != "1782209472" {
		t.Fatalf("secNumber = %q", got)
	}
}

func TestParseAppsTimeAcceptsSupportedInputs(t *testing.T) {
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.Local)
	cases := []struct {
		raw        string
		want       time.Time
		wantOffset *int
	}{
		{raw: "30s", want: now.Add(-30 * time.Second)},
		{raw: "5m", want: now.Add(-5 * time.Minute)},
		{raw: "2h", want: now.Add(-2 * time.Hour)},
		{raw: "1.5h", want: now.Add(-90 * time.Minute)},
		{raw: "0.5d", want: now.Add(-12 * time.Hour)},
		{raw: "3d", want: now.Add(-72 * time.Hour)},
		{raw: "1w", want: now.Add(-7 * 24 * time.Hour)},
		{raw: "2026-06-23", want: time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local)},
		{raw: "2026-06-23T10:11:12", want: time.Date(2026, 6, 23, 10, 11, 12, 0, time.Local)},
		{raw: "2026-06-23T10:11:12.123", want: time.Date(2026, 6, 23, 10, 11, 12, 123000000, time.Local)},
		{raw: "2026-06-23T10:11:12Z", want: time.Date(2026, 6, 23, 10, 11, 12, 0, time.UTC), wantOffset: ptrInt(0)},
		{raw: "2026-06-23T10:11:12+08:00", want: time.Date(2026, 6, 23, 10, 11, 12, 0, time.FixedZone("", 8*60*60)), wantOffset: ptrInt(8 * 60 * 60)},
	}
	for _, tc := range cases {
		got, err := parseAppsTimeFlag("--since", tc.raw, now)
		if err != nil {
			t.Fatalf("parseAppsTimeFlag(%q) err=%v", tc.raw, err)
		}
		if !got.Equal(tc.want) {
			t.Fatalf("parseAppsTimeFlag(%q)=%s, want %s", tc.raw, got.Format(time.RFC3339Nano), tc.want.Format(time.RFC3339Nano))
		}
		if tc.wantOffset != nil {
			_, offset := got.Zone()
			if offset != *tc.wantOffset {
				t.Fatalf("parseAppsTimeFlag(%q) zone offset=%d, want %d", tc.raw, offset, *tc.wantOffset)
			}
		}
	}
}

func TestParseAppsTimeRejectsUnsupportedInputs(t *testing.T) {
	for _, in := range []string{"2026/06/23", "yesterday", "2026-06-23 10:11:12", "999999999999999999w", "2147483647w"} {
		_, _, _, _, err := parseAppsTimeRange("--since", in, "--until", "")
		requireAppsValidationParam(t, err, "--since")
	}
}

func TestParseAppsTimeRangeRejectsSinceAfterUntil(t *testing.T) {
	_, _, _, _, err := parseAppsTimeRange("--since", "2026-06-24", "--until", "2026-06-23")
	requireAppsValidationParam(t, err, "--until")
}

func ptrInt(n int) *int {
	return &n
}
