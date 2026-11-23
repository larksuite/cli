// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"reflect"
	"testing"
)

func TestBuildMeetingEventTimeline_DisambiguatesTranscriptSpeakers(t *testing.T) {
	event := map[string]interface{}{
		"event_type": "transcript_received",
		"payload": map[string]interface{}{
			"transcript_received_items": []interface{}{
				map[string]interface{}{"speaker": map[string]interface{}{"id": "u1", "user_name": "Alice"}, "text": "one"},
				map[string]interface{}{"speaker": map[string]interface{}{"id": "u2", "user_name": "Alice"}, "text": "two"},
				map[string]interface{}{"speaker": map[string]interface{}{"id": "u1", "user_name": "Alice"}, "text": "three"},
				map[string]interface{}{"speaker": map[string]interface{}{"id": "u3", "user_name": "Bob"}, "text": "four"},
				map[string]interface{}{"speaker": map[string]interface{}{"id": "u4"}, "text": "five"},
			},
		},
	}

	timeline := buildMeetingEventTimeline([]interface{}{event})
	var got []string
	for _, entry := range timeline.entries {
		got = append(got, entry.subject)
	}
	want := []string{"Alice[1]", "Alice[2]", "Alice[1]", "Bob", "u4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subjects = %#v, want %#v", got, want)
	}
}
