// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestParseAndValidateSendTime_Empty(t *testing.T) {
	result, err := parseAndValidateSendTime("")
	if err != nil {
		t.Fatalf("expected no error for empty string, got %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty result for empty string, got %q", result)
	}
}

func TestParseAndValidateSendTime_ValidFutureTimestamp(t *testing.T) {
	future := time.Now().Add(10 * time.Minute).Unix()
	input := fmt.Sprintf("%d", future)
	result, err := parseAndValidateSendTime(input)
	if err != nil {
		t.Fatalf("expected no error for valid future timestamp, got %v", err)
	}
	if result != input {
		t.Fatalf("expected result=%q, got %q", input, result)
	}
}

func TestParseAndValidateSendTime_TooSoon(t *testing.T) {
	nearFuture := time.Now().Add(2 * time.Minute).Unix()
	input := fmt.Sprintf("%d", nearFuture)
	_, err := parseAndValidateSendTime(input)
	if err == nil {
		t.Fatal("expected error for timestamp less than 5 minutes in the future")
	}
	if !strings.Contains(err.Error(), "at least 5 minutes") {
		t.Fatalf("expected error about minimum time, got %v", err)
	}
}

func TestParseAndValidateSendTime_PastTimestamp(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).Unix()
	input := fmt.Sprintf("%d", past)
	_, err := parseAndValidateSendTime(input)
	if err == nil {
		t.Fatal("expected error for past timestamp")
	}
}

func TestParseAndValidateSendTime_InvalidFormat(t *testing.T) {
	_, err := parseAndValidateSendTime("not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric input")
	}
	if !strings.Contains(err.Error(), "Unix timestamp") {
		t.Fatalf("expected error about Unix timestamp format, got %v", err)
	}
}

func TestFormatScheduledTimeHuman_ValidTimestamp(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).Unix()
	input := fmt.Sprintf("%d", future)
	result := formatScheduledTimeHuman(input)
	if result == input {
		t.Fatalf("expected human-readable format, got raw timestamp %q", result)
	}
	if !strings.Contains(result, "in ") {
		t.Fatalf("expected relative time in output, got %q", result)
	}
}

func TestFormatScheduledTimeHuman_InvalidTimestamp(t *testing.T) {
	result := formatScheduledTimeHuman("invalid")
	if result != "invalid" {
		t.Fatalf("expected raw fallback for invalid input, got %q", result)
	}
}

func TestFormatScheduledTimeHuman_FarFuture(t *testing.T) {
	future := time.Now().Add(72 * time.Hour).Unix()
	input := fmt.Sprintf("%d", future)
	result := formatScheduledTimeHuman(input)
	if !strings.Contains(result, "days") {
		t.Fatalf("expected 'days' in result, got %q", result)
	}
}

func TestFormatScheduledTimeHuman_MinutesFuture(t *testing.T) {
	future := time.Now().Add(30 * time.Minute).Unix()
	input := fmt.Sprintf("%d", future)
	result := formatScheduledTimeHuman(input)
	if !strings.Contains(result, "in ") || !strings.Contains(result, "minutes") {
		t.Fatalf("expected 'in N minutes' in result, got %q", result)
	}
}
