// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/output"
)

const (
	// minScheduleLeadTime is the minimum time in the future for scheduled send.
	minScheduleLeadTime = 5 * time.Minute
)

// parseAndValidateSendTime parses and validates the --send-time flag value.
// The value is expected to be a Unix timestamp in seconds (string).
// Returns the validated send-time string, or an error if invalid.
// If sendTimeStr is empty, returns ("", nil) indicating immediate send.
func parseAndValidateSendTime(sendTimeStr string) (string, error) {
	if sendTimeStr == "" {
		return "", nil
	}

	ts, err := strconv.ParseInt(sendTimeStr, 10, 64)
	if err != nil {
		return "", output.ErrValidation(
			"--send-time must be a valid Unix timestamp in seconds, got %q", sendTimeStr,
		)
	}

	scheduledTime := time.Unix(ts, 0)
	if time.Until(scheduledTime) < minScheduleLeadTime {
		return "", output.ErrValidation(
			"Scheduled time must be at least 5 minutes in the future",
		)
	}

	return sendTimeStr, nil
}

// formatScheduledTimeHuman returns a human-readable scheduled time string
// for pretty output, e.g. "2026-04-14 09:00:00 UTC (Mon, in 14 hours)"
func formatScheduledTimeHuman(sendTime string) string {
	ts, err := strconv.ParseInt(sendTime, 10, 64)
	if err != nil {
		return sendTime
	}
	t := time.Unix(ts, 0).UTC()
	dur := time.Until(t)
	var relative string
	switch {
	case dur < time.Hour:
		relative = fmt.Sprintf("in %d minutes", int(dur.Minutes()))
	case dur < 24*time.Hour:
		relative = fmt.Sprintf("in %d hours", int(dur.Hours()))
	default:
		relative = fmt.Sprintf("in %d days", int(dur.Hours()/24))
	}
	return fmt.Sprintf("%s (%s, %s)", t.Format(time.RFC3339), t.Format("Mon"), relative)
}
