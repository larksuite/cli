// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFreebusyRSVPMatching(t *testing.T) {
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)
	const valid = `{"ok":true,"data":{"users":[{"user_id":"ou_test","raw_busy":[{"start_time":"2026-01-01T10:00:00Z","end_time":"2026-01-01T10:30:00Z","rsvp_status":"tentative"}]}]}}`
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{"matching raw event", valid, true},
		{"different user", strings.ReplaceAll(valid, "ou_test", "ou_other"), false},
		{"different RSVP", strings.ReplaceAll(valid, "tentative", "accept"), false},
		{"different time", strings.ReplaceAll(valid, "10:30:00Z", "11:00:00Z"), false},
		{"invalid time", strings.ReplaceAll(valid, "2026-01-01T10:00:00Z", "invalid"), false},
		{"merged busy view", strings.ReplaceAll(valid, "raw_busy", "busy"), false},
		{"legacy data array", `{"data":[{"start_time":"2026-01-01T10:00:00Z","end_time":"2026-01-01T10:30:00Z","rsvp_status":"tentative"}]}`, false},
		{"no events", `{"data":{"users":[{"user_id":"ou_test","raw_busy":[]}]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasFreebusyEntry(tc.body, "ou_test", start, end, "tentative"))
		})
	}
}
