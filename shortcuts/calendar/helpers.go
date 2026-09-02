// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const (
	PrimaryCalendarIDStr = "primary"
)

// resolveStartEnd returns (startInput, endInput) from flags with defaults.
// --start defaults to today's date, --end defaults to start date (will be resolved to end-of-day by caller).
func resolveStartEnd(runtime *common.RuntimeContext) (string, string) {
	startInput := runtime.Str("start")
	if startInput == "" {
		startInput = time.Now().Format("2006-01-02")
	}
	endInput := runtime.Str("end")
	if endInput == "" {
		endInput = startInput
	}
	return startInput, endInput
}

func collapseDescription(event map[string]interface{}) {
	if event == nil {
		return
	}
	rich, _ := event["description_rich"].(string)
	plain, _ := event["description"].(string)
	delete(event, "description_rich")
	switch {
	case rich != "":
		event["description"] = rich
	case plain != "":
		event["description"] = plain
	default:
		delete(event, "description")
	}
}
func descriptionToSend(runtime *common.RuntimeContext) string {
	return runtime.Str("description")
}

func hasExplicitBotFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flag("as")
	return flag != nil && flag.Changed && flag.Value != nil && strings.TrimSpace(flag.Value.String()) == "bot"
}

func rejectCalendarAutoBotFallback(runtime *common.RuntimeContext) error {
	if runtime == nil || !runtime.IsBot() || hasExplicitBotFlag(runtime.Cmd) {
		return nil
	}
	if runtime.Factory == nil || !runtime.Factory.IdentityAutoDetected {
		return nil
	}

	message := recovery.Join("",
		recovery.Text("calendar commands require a valid user login by default; when no valid user login state is available, auto identity falls back to bot and may operate on the bot calendar instead of your own. "),
		recovery.Command(recovery.TargetAuthLogin,
			"Run `lark-cli auth login --domain calendar` for your calendar, "),
		recovery.Text("or rerun with `--as bot` if bot identity is intentional."),
	)
	hint := recovery.Join("\n",
		recovery.Command(recovery.TargetAuthLogin,
			"restore user login: `lark-cli auth login --domain calendar`"),
		recovery.Text("intentional bot usage: rerun with `--as bot`"),
	)
	err := recovery.Attach(
		errs.NewAuthenticationError(errs.SubtypeTokenMissing, "%s", message.String()),
		hint,
	)
	return recovery.AnnotateMessage(err, message)
}

// calendarTimeInputRange declares a single time input to check for offset drift.
// A start/end pair is passed as two separate entries so each label is reported
// independently.
type calendarTimeInputRange struct {
	Flag  string
	Value string
}

// calendarTimezoneMismatch captures one input whose explicit offset differs
// from the local system offset at the same instant, together with the values
// needed to render a self-explanatory line to the user.
type calendarTimezoneMismatch struct {
	flag      string
	value     string
	inputZone string
	localZone string
}

// warnCalendarTimezoneMismatch inspects each provided time input and, when the
// input carries an explicit UTC offset that differs from the current system
// local offset at that instant, writes a single non-blocking hint to stderr.
// It never returns an error — the goal is to catch accidental cross-timezone
// scheduling before an event lands on the wrong wall clock.
//
// Inputs without an explicit offset (bare date, "YYYY-MM-DDTHH:MM(:SS)" with no
// zone, Unix timestamps, empty strings, unparseable strings) are skipped: the
// user has not asserted a specific zone, so there is nothing to reconcile.
func warnCalendarTimezoneMismatch(runtime *common.RuntimeContext, inputs ...calendarTimeInputRange) {
	if runtime == nil || runtime.Factory == nil || runtime.Factory.IOStreams == nil {
		return
	}
	stderr := runtime.Factory.IOStreams.ErrOut
	if stderr == nil {
		return
	}
	var mismatches []calendarTimezoneMismatch
	seen := map[string]struct{}{}
	for _, in := range inputs {
		val := strings.TrimSpace(in.Value)
		if val == "" {
			continue
		}
		t, ok := parseTimeWithExplicitOffset(val)
		if !ok {
			continue
		}
		_, inOffset := t.Zone()
		_, localOffset := t.In(time.Local).Zone()
		if inOffset == localOffset {
			continue
		}
		key := in.Flag + "|" + val
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		mismatches = append(mismatches, calendarTimezoneMismatch{
			flag:      in.Flag,
			value:     val,
			inputZone: formatTimezoneOffset(inOffset),
			localZone: formatTimezoneOffset(localOffset),
		})
	}
	if len(mismatches) == 0 {
		return
	}
	writeCalendarTimezoneMismatchHint(stderr, mismatches)
}

// writeCalendarTimezoneMismatchHint renders the shared hint header plus one
// line per mismatched input. Kept separate from the collection loop so the
// message format stays in one place. Only the timezone labels are shown —
// the wall-clock digits do not change when relabeling an instant into another
// zone, so printing a "local time" column would misleadingly imply otherwise.
func writeCalendarTimezoneMismatchHint(w io.Writer, mismatches []calendarTimezoneMismatch) {
	fmt.Fprintf(w,
		"hint: the timezone in the provided time differs from the local system timezone (local: %s); if the user explicitly requested that timezone, ignore this hint — otherwise prefer the local timezone.\n",
		mismatches[0].localZone)
}

// parseTimeWithExplicitOffset returns the parsed time when input is an ISO 8601
// string that carries an explicit UTC offset (e.g. "+08:00", "-05:00", or "Z").
// Inputs without an explicit offset return (_, false) so the caller can skip
// them: the user has not claimed a zone, so there is no mismatch to detect.
func parseTimeWithExplicitOffset(input string) (time.Time, bool) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04Z07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, input); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// collectCalendarRangeInputs splits a comma-separated list of "start~end" pairs
// (as used by --exclude / --slot) into individual entries suitable for
// warnCalendarTimezoneMismatch. Malformed pairs are skipped silently because
// the calling shortcut's Validate step already surfaces those as errors.
func collectCalendarRangeInputs(flag, raw string) []calendarTimeInputRange {
	var out []calendarTimeInputRange
	for _, r := range strings.Split(strings.TrimSpace(raw), ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		parts := strings.SplitN(r, "~", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out,
			calendarTimeInputRange{Flag: flag, Value: strings.TrimSpace(parts[0])},
			calendarTimeInputRange{Flag: flag, Value: strings.TrimSpace(parts[1])},
		)
	}
	return out
}

// formatTimezoneOffset renders an offset in seconds as "UTC+8" / "UTC-5:30" /
// "UTC". Minute precision is included only when the offset has a non-zero
// minute component so the common whole-hour case stays terse.
func formatTimezoneOffset(offsetSec int) string {
	if offsetSec == 0 {
		return "UTC"
	}
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	h := offsetSec / 3600
	m := (offsetSec % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("UTC%s%d", sign, h)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, h, m)
}
