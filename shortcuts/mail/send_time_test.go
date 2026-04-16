// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"
	"time"
)

func TestParseAndValidateSendTime(t *testing.T) {
	now := time.Now()

	t.Run("rfc3339", func(t *testing.T) {
		input := now.Add(10 * time.Minute).UTC().Format(time.RFC3339)
		got, err := parseAndValidateSendTime(input)
		if err != nil {
			t.Fatalf("parseAndValidateSendTime() error = %v", err)
		}
		if got != input {
			t.Fatalf("parseAndValidateSendTime() = %q, want %q", got, input)
		}
	})

	t.Run("no timezone defaults to utc", func(t *testing.T) {
		target := now.Add(10 * time.Minute).UTC()
		input := target.Format("2006-01-02T15:04:05")
		want := target.Format(time.RFC3339)
		got, err := parseAndValidateSendTime(input)
		if err != nil {
			t.Fatalf("parseAndValidateSendTime() error = %v", err)
		}
		if got != want {
			t.Fatalf("parseAndValidateSendTime() = %q, want %q", got, want)
		}
	})

	t.Run("too soon", func(t *testing.T) {
		input := now.Add(4 * time.Minute).UTC().Format(time.RFC3339)
		_, err := parseAndValidateSendTime(input)
		if err == nil {
			t.Fatal("parseAndValidateSendTime() expected error for too-soon time")
		}
		if !strings.Contains(err.Error(), "5 minutes") {
			t.Fatalf("expected 5-minute guard error, got %v", err)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		_, err := parseAndValidateSendTime("not-a-time")
		if err == nil {
			t.Fatal("parseAndValidateSendTime() expected error for invalid input")
		}
		if !strings.Contains(err.Error(), "RFC3339") {
			t.Fatalf("expected RFC3339 hint, got %v", err)
		}
	})
}
