// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"testing"

	event "github.com/larksuite/cli/internal/event"

	_ "github.com/larksuite/cli/events"
)

// expectedKeys is the frozen catalog baseline. Adding, removing, or renaming
// an EventKey is a deliberate contract change: update this list in the same
// commit and call the change out in the changelog.
var expectedKeys = []string{
	"application.bot.menu_v6",
	"approval.instance.status_changed_v4",
	"approval.task.status_changed_v4",
	"board.whiteboard.updated_v1",
	"card.action.trigger",
	"im.chat.disbanded_v1",
	"im.chat.member.bot.added_v1",
	"im.chat.member.bot.deleted_v1",
	"im.chat.member.user.added_v1",
	"im.chat.member.user.deleted_v1",
	"im.chat.member.user.withdrawn_v1",
	"im.chat.updated_v1",
	"im.message.message_read_v1",
	"im.message.reaction.created_v1",
	"im.message.reaction.deleted_v1",
	"im.message.receive_v1",
	"minutes.minute.generated_v1",
	"task.task.update_user_access_v2",
	"vc.meeting.participant_meeting_ended_v1",
	"vc.meeting.participant_meeting_joined_v1",
	"vc.meeting.participant_meeting_started_v1",
	"vc.note.generated_v1",
	"vc.recording.recording_ended_v1",
	"vc.recording.recording_started_v1",
	"vc.recording.recording_transcript_generated_v1",
}

func TestRegisteredKeys_MatchFrozenBaseline(t *testing.T) {
	all := event.ListAll()
	if len(all) == 0 {
		t.Fatal("no EventKeys registered; the gate scanned nothing")
	}
	got := make(map[string]bool, len(all))
	for _, def := range all {
		got[def.Key] = true
	}
	for _, want := range expectedKeys {
		if !got[want] {
			t.Errorf("expected EventKey missing from registry: %s", want)
		}
		delete(got, want)
	}
	for extra := range got {
		t.Errorf("EventKey not in frozen baseline (update expectedKeys deliberately): %s", extra)
	}
}
