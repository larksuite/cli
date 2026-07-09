// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"encoding/json"
	"strings"
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

func TestCollectRoomFindResults_EmptySlotEmitsHintAndArray(t *testing.T) {
	slots := []roomFindSlot{
		{Start: "2026-03-27T14:00:00+08:00", End: "2026-03-27T15:00:00+08:00"},
		{Start: "2026-03-27T15:00:00+08:00", End: "2026-03-27T16:00:00+08:00"},
	}

	out, err := collectRoomFindResults(slots, 2, func(slot roomFindSlot) ([]*roomFindSuggestion, error) {
		if strings.HasPrefix(slot.Start, "2026-03-27T14") {
			return []*roomFindSuggestion{{RoomID: "rm_1", RoomName: "Room A"}}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("collectRoomFindResults returned error: %v", err)
	}
	if len(out.TimeSlots) != 2 {
		t.Fatalf("expected 2 time slots, got %d", len(out.TimeSlots))
	}

	for _, ts := range out.TimeSlots {
		if ts.MeetingRooms == nil {
			t.Fatalf("meeting_rooms should be non-nil for slot %s", ts.Start)
		}
		switch {
		case strings.HasPrefix(ts.Start, "2026-03-27T14"):
			if len(ts.MeetingRooms) != 1 {
				t.Fatalf("expected 1 room for first slot, got %d", len(ts.MeetingRooms))
			}
			if ts.Hint != "" {
				t.Fatalf("non-empty slot should not carry hint, got %q", ts.Hint)
			}
		case strings.HasPrefix(ts.Start, "2026-03-27T15"):
			if len(ts.MeetingRooms) != 0 {
				t.Fatalf("expected 0 rooms for empty slot, got %d", len(ts.MeetingRooms))
			}
			if ts.Hint == "" {
				t.Fatal("empty slot should carry a hint explaining the filters")
			}
		}
	}

	emptySlot := out.TimeSlots[0]
	if !strings.HasPrefix(emptySlot.Start, "2026-03-27T15") {
		emptySlot = out.TimeSlots[1]
	}
	raw, err := json.Marshal(emptySlot)
	if err != nil {
		t.Fatalf("marshal empty slot: %v", err)
	}
	if !strings.Contains(string(raw), `"meeting_rooms":[]`) {
		t.Fatalf("expected meeting_rooms:[] in JSON, got %s", raw)
	}
	if !strings.Contains(string(raw), `"hint"`) {
		t.Fatalf("expected hint field in JSON, got %s", raw)
	}
}
