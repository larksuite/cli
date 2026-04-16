// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
)

const minScheduledSendLeadTime = 5 * time.Minute

// parseAndValidateSendTime validates --send-time and returns a normalized RFC3339
// string. If the input omits a timezone offset, UTC is assumed.
func parseAndValidateSendTime(sendTimeStr string) (string, error) {
	sendTimeStr = strings.TrimSpace(sendTimeStr)
	if sendTimeStr == "" {
		return "", nil
	}

	t, err := time.Parse(time.RFC3339, sendTimeStr)
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02T15:04:05", sendTimeStr, time.UTC)
		if err != nil {
			return "", output.ErrValidation(
				"--send-time must be RFC3339, for example 2026-04-14T09:00:00+08:00; if you omit the timezone, UTC is assumed",
			)
		}
	}

	if time.Until(t) < minScheduledSendLeadTime {
		return "", output.ErrValidation("--send-time must be at least 5 minutes in the future")
	}

	return t.UTC().Format(time.RFC3339), nil
}

// formatScheduledTimeHuman renders a scheduled send time with a short relative hint.
func formatScheduledTimeHuman(sendTime string) string {
	t, err := time.Parse(time.RFC3339, sendTime)
	if err != nil {
		return sendTime
	}
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
	return fmt.Sprintf("%s (%s, %s)", sendTime, t.Format("Mon"), relative)
}
