// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWarnCalendarTimezoneMismatch_MatchingOffsetSilent(t *testing.T) {
	f, _, stderrBuf, _ := cmdutil.TestFactory(t, defaultConfig())
	rt := &common.RuntimeContext{Factory: f}

	_, localOffset := time.Now().Zone()
	val := time.Now().Format("2006-01-02T15:04:05") + formatOffsetSuffix(localOffset)

	warnCalendarTimezoneMismatch(rt, calendarTimeInputRange{Flag: "start", Value: val})
	if got := stderrBuf.String(); got != "" {
		t.Fatalf("expected no warning for matching offset, got: %q", got)
	}
}

func TestWarnCalendarTimezoneMismatch_MismatchedOffsetWarns(t *testing.T) {
	f, _, stderrBuf, _ := cmdutil.TestFactory(t, defaultConfig())
	rt := &common.RuntimeContext{Factory: f}

	// Shift the local offset by 1 hour so it's guaranteed different; fold back
	// if the shift lands outside the valid IANA range.
	_, localOffset := time.Now().Zone()
	shifted := localOffset + 3600
	if shifted >= 14*3600 {
		shifted = localOffset - 3600
	}
	val := "2026-03-21T09:00:00" + formatOffsetSuffix(shifted)

	warnCalendarTimezoneMismatch(rt, calendarTimeInputRange{Flag: "start", Value: val})
	got := stderrBuf.String()
	if !strings.Contains(got, "local system timezone") || !strings.Contains(got, "(local: UTC") {
		t.Fatalf("expected timezone mismatch hint with local zone on stderr, got: %q", got)
	}
	if !strings.Contains(got, "prefer the local timezone") || !strings.Contains(got, "ignore this hint") {
		t.Fatalf("expected guidance to prefer local timezone with ignore-if-intentional escape, got: %q", got)
	}
}

func TestWarnCalendarTimezoneMismatch_NoExplicitOffsetSkipped(t *testing.T) {
	f, _, stderrBuf, _ := cmdutil.TestFactory(t, defaultConfig())
	rt := &common.RuntimeContext{Factory: f}

	warnCalendarTimezoneMismatch(rt,
		calendarTimeInputRange{Flag: "start", Value: "2026-03-21T09:00:00"},
		calendarTimeInputRange{Flag: "end", Value: "2026-03-21"},
		calendarTimeInputRange{Flag: "start", Value: "1710982800"},
		calendarTimeInputRange{Flag: "end", Value: ""},
	)
	if got := stderrBuf.String(); got != "" {
		t.Fatalf("expected no warning for inputs without explicit offset, got: %q", got)
	}
}

func TestCollectCalendarRangeInputs_SplitsPairs(t *testing.T) {
	out := collectCalendarRangeInputs("exclude",
		"2026-03-21T09:00:00+08:00~2026-03-21T10:00:00+08:00, 2026-03-22T09:00:00Z~2026-03-22T10:00:00Z")
	if len(out) != 4 {
		t.Fatalf("expected 4 entries, got %d: %#v", len(out), out)
	}
	for _, e := range out {
		if e.Flag != "exclude" {
			t.Errorf("expected flag=exclude, got %q", e.Flag)
		}
	}
}

func TestFormatTimezoneOffset(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "UTC"},
		{8 * 3600, "UTC+8"},
		{-5 * 3600, "UTC-5"},
		{5*3600 + 30*60, "UTC+5:30"},
		{-3*3600 - 30*60, "UTC-3:30"},
	}
	for _, c := range cases {
		if got := formatTimezoneOffset(c.in); got != c.want {
			t.Errorf("formatTimezoneOffset(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// formatOffsetSuffix renders a signed offset as the "+HH:MM" / "-HH:MM"
// suffix accepted by ISO 8601 parsers.
func formatOffsetSuffix(offsetSec int) string {
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	return fmt.Sprintf("%s%02d:%02d", sign, offsetSec/3600, (offsetSec%3600)/60)
}
