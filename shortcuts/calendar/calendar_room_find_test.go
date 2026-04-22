// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"testing"
	"time"
)

func TestNormalizeCommaSeparatedNames(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"木星", "木星"},
		{"01,02,03", "01,02,03"},
		{" 01 , 02 , 03 ", "01,02,03"},
		{"16,17,18,19,20", "16,17,18,19,20"},
		{"", ""},
		{" , , ", ""},
		{"01,,03", "01,03"},
		{" 木星 ", "木星"},
	}
	for _, tt := range tests {
		got := normalizeCommaSeparatedNames(tt.input)
		if got != tt.want {
			t.Errorf("normalizeCommaSeparatedNames(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCollectRoomFindResults_LimitsConcurrency(t *testing.T) {
	slots := []roomFindSlot{
		{Start: "2026-03-27T14:00:00+08:00", End: "2026-03-27T15:00:00+08:00"},
		{Start: "2026-03-27T15:00:00+08:00", End: "2026-03-27T16:00:00+08:00"},
		{Start: "2026-03-27T16:00:00+08:00", End: "2026-03-27T17:00:00+08:00"},
	}

	entered := make(chan struct{}, len(slots))
	release := make(chan struct{})
	done := make(chan *roomFindOutput, 1)
	errCh := make(chan error, 1)

	go func() {
		out, err := collectRoomFindResults(slots, 2, func(slot roomFindSlot) ([]*roomFindSuggestion, error) {
			entered <- struct{}{}
			<-release
			return []*roomFindSuggestion{{RoomName: slot.Start}}, nil
		})
		errCh <- err
		done <- out
	}()

	for range 2 {
		select {
		case <-entered:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("timed out waiting for room-find workers to start")
		}
	}

	select {
	case <-entered:
		t.Fatal("room-find exceeded the configured concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("collectRoomFindResults returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for room-find results")
	}

	out := <-done
	if len(out.TimeSlots) != len(slots) {
		t.Fatalf("expected %d time slots, got %d", len(slots), len(out.TimeSlots))
	}
}
