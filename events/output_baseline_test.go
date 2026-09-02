// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	event "github.com/larksuite/cli/internal/event"
)

var updateBaseline = flag.Bool("update-baseline", false,
	"rewrite testdata/output_baseline.json with the current Processed EventKey outputs")

// TestMain pins the process timezone to UTC before any test runs. Several
// Process handlers format timestamps in the machine's local timezone
// (e.g. meeting start/end times, recording event times), so without the pin
// the snapshot would drift between machines in different timezones.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

const baselineSnapshotPath = "testdata/output_baseline.json"

// wantProcessedKeys freezes how many registered EventKeys define Process
// (im 2, vc 7, minutes 1, application 1, approval 2). The count assertion
// keeps this test honest: if a Processed key is added or removed, the covered
// output surface changed and the baseline would silently widen or narrow
// without it. Update the count, the fixtures, and the snapshot together,
// deliberately.
const wantProcessedKeys = 13

const (
	baselineEventID    = "evt-baseline-001"
	baselineCreateTime = "1700000000000" // 2023-11-14T22:13:20Z in milliseconds
)

// baselineFixture holds the minimal well-formed inputs for one Processed
// EventKey: the business body placed under "event" in the V2 envelope, plus
// any extra header fields the handler reads beyond event_id / event_type /
// create_time. Every fixture must drive Process down its success path — no
// drop, no malformed-payload passthrough.
type baselineFixture struct {
	extraHeader map[string]string
	eventBody   string
}

// baselineFixtures maps every Processed EventKey to its synthetic input.
// Field values are fixed constants so the resulting output is byte-stable.
var baselineFixtures = map[string]baselineFixture{
	"application.bot.menu_v6": {
		extraHeader: map[string]string{
			"app_id":     "cli-baseline-app",
			"tenant_key": "tenant-baseline",
		},
		// 10-digit seconds timestamp: the handler normalizes it to milliseconds.
		eventBody: `{
			"event_key": "baseline_menu_key",
			"timestamp": 1700000000,
			"operator": {
				"operator_id": {
					"open_id": "ou-baseline-operator",
					"union_id": "on-baseline-operator",
					"user_id": "user-baseline-operator"
				},
				"operator_name": "Baseline Operator"
			}
		}`,
	},
	"approval.instance.status_changed_v4": {
		eventBody: `{
			"approval_code": "approval-code-baseline",
			"instance_code": "instance-code-baseline",
			"external_id": "external-id-baseline",
			"status": "APPROVED",
			"operate_time": "1700000000000",
			"start_user": {
				"open_id": "ou-baseline-starter",
				"union_id": "on-baseline-starter",
				"user_id": "user-baseline-starter"
			}
		}`,
	},
	"approval.task.status_changed_v4": {
		eventBody: `{
			"approval_code": "approval-code-baseline",
			"instance_code": "instance-code-baseline",
			"task_id": "task-id-baseline",
			"external_id": "external-id-baseline",
			"task_external_id": "task-external-id-baseline",
			"status": "APPROVED",
			"operate_time": "1700000000000",
			"assigned_user": {
				"open_id": "ou-baseline-assignee",
				"union_id": "on-baseline-assignee",
				"user_id": "user-baseline-assignee"
			}
		}`,
	},
	// The card handler fetches the card content through the API client using
	// context.open_message_id; the fake client below serves that request.
	"card.action.trigger": {
		eventBody: `{
			"operator": {"open_id": "ou-baseline-operator"},
			"token": "card-token-baseline",
			"host": "im_message",
			"action": {
				"tag": "button",
				"value": {"key": "baseline"},
				"name": "baseline_button",
				"form_value": {"field": "value"},
				"input_value": "baseline input",
				"option": "opt-1",
				"options": ["opt-1", "opt-2"],
				"checked": true,
				"timezone": "Asia/Shanghai"
			},
			"context": {
				"open_message_id": "om-baseline-card",
				"open_chat_id": "oc-baseline-chat"
			}
		}`,
	},
	// update_time differs from create_time so the handler emits both; the
	// mention placeholder in content exercises mention rendering.
	"im.message.receive_v1": {
		eventBody: `{
			"sender": {
				"sender_type": "user",
				"sender_id": {"open_id": "ou-baseline-sender"}
			},
			"message": {
				"message_id": "om-baseline-msg",
				"root_id": "om-baseline-root",
				"parent_id": "om-baseline-parent",
				"thread_id": "omt-baseline-thread",
				"chat_id": "oc-baseline-chat",
				"chat_type": "p2p",
				"message_type": "text",
				"create_time": "1699999999000",
				"update_time": "1700000000500",
				"content": "{\"text\":\"hello @_user_1\"}",
				"mentions": [
					{
						"key": "@_user_1",
						"id": {"open_id": "ou-baseline-mention"},
						"name": "Baseline User"
					}
				]
			}
		}`,
	},
	// The minutes handler enriches the output with the minute title via the
	// API client; the fake client answers with a non-empty title on the first
	// call so no retry attempt is made.
	"minutes.minute.generated_v1": {
		eventBody: `{
			"minute_token": "minute-token-baseline",
			"minute_source": {
				"source_type": "meeting",
				"source_entity_id": "meeting-entity-baseline"
			}
		}`,
	},
	"vc.meeting.participant_meeting_started_v1": {
		eventBody: `{
			"meeting": {
				"id": "meeting-id-baseline",
				"topic": "Baseline meeting",
				"meeting_no": "123456789",
				"start_time": "1700000000",
				"calendar_event_id": "calendar-event-baseline"
			}
		}`,
	},
	"vc.meeting.participant_meeting_joined_v1": {
		eventBody: `{
			"meeting": {
				"id": "meeting-id-baseline",
				"topic": "Baseline meeting",
				"meeting_no": "123456789",
				"start_time": "1700000000",
				"calendar_event_id": "calendar-event-baseline"
			}
		}`,
	},
	"vc.meeting.participant_meeting_ended_v1": {
		eventBody: `{
			"meeting": {
				"id": "meeting-id-baseline",
				"topic": "Baseline meeting",
				"meeting_no": "123456789",
				"start_time": "1700000000",
				"end_time": "1700000600",
				"calendar_event_id": "calendar-event-baseline"
			}
		}`,
	},
	// The note handler enriches the output with document tokens via the API
	// client; the fake client answers with both artifacts on the first call
	// so no retry attempt is made.
	"vc.note.generated_v1": {
		eventBody: `{"note_id": "note-id-baseline"}`,
	},
	// Recording handlers only emit events whose source is recording_bean;
	// anything else is dropped, which would break the success-path contract.
	"vc.recording.recording_started_v1": {
		eventBody: `{
			"unique_key": "recording-key-baseline",
			"source": "recording_bean"
		}`,
	},
	"vc.recording.recording_transcript_generated_v1": {
		eventBody: `{
			"unique_key": "recording-key-baseline",
			"source": "recording_bean",
			"transcript_items": [
				{
					"speaker": {"user_name": "Baseline Speaker"},
					"text": "baseline transcript text",
					"start_time_ms": "1700000000000",
					"end_time_ms": "1700000001000",
					"sentence_id": "sentence-baseline-1"
				}
			]
		}`,
	},
	"vc.recording.recording_ended_v1": {
		eventBody: `{
			"unique_key": "recording-key-baseline",
			"source": "recording_bean"
		}`,
	},
}

// baselineAPIResponses maps request paths to canned success responses for the
// handlers that call the API during Process. Every response satisfies the
// handler on the first call, so retry loops never engage and no real network
// or credentials are involved.
var baselineAPIResponses = map[string]string{
	"/open-apis/im/v1/messages/om-baseline-card?card_msg_content_type=user_card_content": `{
		"code": 0,
		"msg": "success",
		"data": {
			"items": [
				{"body": {"content": "{\"header\":{\"title\":{\"tag\":\"plain_text\",\"content\":\"Baseline card\"}}}"}}
			]
		}
	}`,
	"/open-apis/vc/v1/notes/note-id-baseline": `{
		"code": 0,
		"msg": "success",
		"data": {
			"note": {
				"artifacts": [
					{"artifact_type": 1, "doc_token": "note-doc-token-baseline"},
					{"artifact_type": 2, "doc_token": "verbatim-doc-token-baseline"}
				],
				"note_source": {
					"source_type": "meeting",
					"source_entity_id": "meeting-entity-baseline"
				}
			}
		}
	}`,
	"/open-apis/minutes/v1/minutes/minute-token-baseline": `{
		"code": 0,
		"msg": "success",
		"data": {
			"minute": {"title": "Baseline minute title"}
		}
	}`,
}

// baselineAPIClient serves the canned responses above. An unexpected request
// path fails the test immediately instead of returning an error, because
// several handlers swallow API errors (or retry with delays) and would
// silently produce a degraded output that gets frozen into the baseline.
type baselineAPIClient struct {
	t *testing.T
}

func (c *baselineAPIClient) CallAPI(_ context.Context, method, path string, _ any) (json.RawMessage, error) {
	c.t.Helper()
	resp, ok := baselineAPIResponses[path]
	if !ok {
		c.t.Fatalf("unexpected API call during Process: %s %s — add a canned response to baselineAPIResponses", method, path)
	}
	return json.RawMessage(resp), nil
}

// TestProcessedOutputBaseline runs every Processed EventKey against a fixed
// well-formed synthetic payload and compares the outputs with the frozen
// snapshot in testdata/output_baseline.json. Any change to what a Processed
// key writes to stdout for a well-formed event shows up here as a named,
// per-key diff. Run with -update-baseline to accept an intentional change.
func TestProcessedOutputBaseline(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	rt := &baselineAPIClient{t: t}
	got := map[string]json.RawMessage{}
	seenFixtures := map[string]bool{}

	for _, def := range compileRealCatalog(t).Definitions() {
		if def.Process == nil {
			continue
		}
		fx, ok := baselineFixtures[def.Key]
		if !ok {
			t.Fatalf("Processed EventKey %q has no baseline fixture; add one to baselineFixtures, bump wantProcessedKeys, and regenerate with -update-baseline", def.Key)
		}
		seenFixtures[def.Key] = true

		payload := buildBaselineEnvelope(t, def.EventType, fx)
		// The canonical fields mirror the synthetic envelope header exactly,
		// including any extra header fields, just as the consume pipeline
		// guarantees for real events before Process runs.
		raw := &event.RawEvent{
			EventID:    baselineEventID,
			EventType:  def.EventType,
			SourceTime: baselineCreateTime,
			AppID:      fx.extraHeader["app_id"],
			TenantKey:  fx.extraHeader["tenant_key"],
			Payload:    payload,
			Timestamp:  time.Unix(1700000000, 0).UTC(),
		}

		out, err := def.Process(context.Background(), rt, raw, nil)
		if err != nil {
			t.Fatalf("%s: Process returned error on well-formed payload: %v", def.Key, err)
		}
		if out == nil {
			t.Fatalf("%s: Process dropped a well-formed payload; the fixture must exercise the success path", def.Key)
		}
		if bytes.Equal(compactJSON(t, def.Key, out), compactJSON(t, def.Key, payload)) {
			t.Fatalf("%s: Process returned the input unchanged; the fixture must exercise the success path, not the malformed-payload passthrough", def.Key)
		}
		got[def.Key] = out
	}

	if len(got) != wantProcessedKeys {
		t.Fatalf("processed %d EventKeys, want exactly %d; a Processed key was added or removed — update baselineFixtures, wantProcessedKeys, and the snapshot together (keys run: %v)",
			len(got), wantProcessedKeys, sortedKeys(got))
	}
	for key := range baselineFixtures {
		if !seenFixtures[key] {
			t.Fatalf("baseline fixture %q matches no registered Processed EventKey; remove it or fix the key name", key)
		}
	}

	if *updateBaseline {
		writeBaselineSnapshot(t, got)
		return
	}
	compareBaselineSnapshot(t, got)
}

// buildBaselineEnvelope wraps a fixture body in the standard V2 event
// envelope with fixed header values.
func buildBaselineEnvelope(t *testing.T, eventType string, fx baselineFixture) json.RawMessage {
	t.Helper()
	header := map[string]string{
		"event_id":    baselineEventID,
		"event_type":  eventType,
		"create_time": baselineCreateTime,
	}
	maps.Copy(header, fx.extraHeader)
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal envelope header: %v", err)
	}
	envelope := map[string]json.RawMessage{
		"schema": json.RawMessage(`"2.0"`),
		"header": headerJSON,
		"event":  json.RawMessage(fx.eventBody),
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope for %s: %v", eventType, err)
	}
	return payload
}

func writeBaselineSnapshot(t *testing.T, got map[string]json.RawMessage) {
	t.Helper()
	// MarshalIndent sorts map keys, so the snapshot is deterministic.
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(baselineSnapshotPath), 0o755); err != nil {
		t.Fatalf("create testdata dir: %v", err)
	}
	if err := os.WriteFile(baselineSnapshotPath, data, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	t.Logf("baseline snapshot rewritten: %s (%d keys)", baselineSnapshotPath, len(got))
}

func compareBaselineSnapshot(t *testing.T, got map[string]json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(baselineSnapshotPath)
	if os.IsNotExist(err) {
		t.Fatalf("baseline snapshot %s not found; generate it with: go test ./events/ -run TestProcessedOutput -update-baseline", baselineSnapshotPath)
	}
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var want map[string]json.RawMessage
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("snapshot %s is not valid JSON: %v", baselineSnapshotPath, err)
	}

	for _, key := range sortedKeys(want) {
		if _, ok := got[key]; !ok {
			t.Errorf("%s: present in snapshot but produced no output this run; if the key was removed on purpose, regenerate with -update-baseline", key)
		}
	}
	for _, key := range sortedKeys(got) {
		wantOut, ok := want[key]
		if !ok {
			t.Errorf("%s: produced output but missing from snapshot; regenerate with -update-baseline", key)
			continue
		}
		gotC := compactJSON(t, key, got[key])
		wantC := compactJSON(t, key, wantOut)
		if !bytes.Equal(gotC, wantC) {
			t.Errorf("%s: Processed output drifted from baseline\n  got:  %s\n  want: %s\nIf this change is intentional, regenerate with -update-baseline", key, gotC, wantC)
		}
	}
}

// compactJSON canonicalizes whitespace so comparisons are content-only.
func compactJSON(t *testing.T, key string, raw json.RawMessage) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatalf("%s: output is not valid JSON: %v\nraw=%s", key, err, string(raw))
	}
	return buf.Bytes()
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
