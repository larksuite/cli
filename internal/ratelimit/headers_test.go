// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"net/http"
	"testing"
	"time"
)

func TestParseHeaders(t *testing.T) {
	now := time.Date(2026, time.August, 6, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		header      http.Header
		wantDelay   time.Duration
		wantSeconds int
		wantLimit   int64
	}{
		{
			name: "ogw reset takes precedence",
			header: http.Header{
				HeaderReset:   {"8"},
				HeaderLimit:   {"100"},
				"Retry-After": {"4"},
			},
			wantDelay:   8 * time.Second,
			wantSeconds: 8,
			wantLimit:   100,
		},
		{name: "standard delta", header: http.Header{"Retry-After": {"4"}}, wantDelay: 4 * time.Second, wantSeconds: 4},
		{name: "standard date", header: http.Header{"Retry-After": {now.Add(1500 * time.Millisecond).Format(http.TimeFormat)}}, wantDelay: time.Second, wantSeconds: 1},
		{name: "invalid reset falls back", header: http.Header{HeaderReset: {"invalid"}, "Retry-After": {"3"}}, wantDelay: 3 * time.Second, wantSeconds: 3},
		{name: "non-positive values", header: http.Header{HeaderReset: {"0"}, HeaderLimit: {"-1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHeaders(tt.header, now)
			if got.RetryAfter != tt.wantDelay || got.RetryAfterSeconds() != tt.wantSeconds || got.Limit != tt.wantLimit {
				t.Fatalf("ParseHeaders() = %+v, seconds=%d; want delay=%s seconds=%d limit=%d", got, got.RetryAfterSeconds(), tt.wantDelay, tt.wantSeconds, tt.wantLimit)
			}
		})
	}
}

func TestRetryAfterSecondsRoundsUp(t *testing.T) {
	info := Info{RetryAfter: 1500 * time.Millisecond}
	if got := info.RetryAfterSeconds(); got != 2 {
		t.Fatalf("RetryAfterSeconds() = %d, want 2", got)
	}
}
