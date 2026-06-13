// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	eventlib "github.com/larksuite/cli/internal/event"

	_ "github.com/larksuite/cli/events"
)

func TestEventLookup_VCMeetingLifecycleKeys(t *testing.T) {
	for _, key := range []string{
		"vc.meeting.participant_meeting_started_v1",
		"vc.meeting.participant_meeting_joined_v1",
	} {
		if _, ok := eventlib.Lookup(key); !ok {
			t.Fatalf("event.Lookup(%q) should succeed", key)
		}
	}
}

func TestRunList_TextOutput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runList(f, false); err != nil {
		t.Fatalf("runList: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"KEY", "AUTH", "PARAMS", "DESCRIPTION",
		"im.message.receive_v1",
		"im.message.message_read_v1",
		"task.task.update_user_access_v2",
		"vc.meeting.participant_meeting_started_v1",
		"vc.meeting.participant_meeting_joined_v1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q; full output:\n%s", want, out)
		}
	}
}

func TestRunList_JSONOutput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runList(f, true); err != nil {
		t.Fatalf("runList json: %v", err)
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one EventKey in JSON output")
	}

	for _, row := range rows {
		for _, field := range []string{"key", "event_type", "schema"} {
			if row[field] == nil {
				t.Errorf("row missing %q: %+v", field, row)
			}
		}
	}

	gotKeys := map[string]map[string]interface{}{}
	for _, row := range rows {
		if key, ok := row["key"].(string); ok {
			gotKeys[key] = row
		}
	}
	var foundTask bool
	for key, row := range gotKeys {
		if key == "task.task.update_user_access_v2" {
			foundTask = true
			if row["single_consumer"] != true {
				t.Errorf("task row single_consumer = %v, want true", row["single_consumer"])
			}
		}
	}
	if !foundTask {
		t.Fatal("event list JSON missing task.task.update_user_access_v2")
	}
	for _, want := range []string{
		"vc.meeting.participant_meeting_started_v1",
		"vc.meeting.participant_meeting_joined_v1",
	} {
		if _, ok := gotKeys[want]; !ok {
			t.Errorf("JSON list output missing %q", want)
		}
	}
}
