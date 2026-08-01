// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	appconsume "github.com/larksuite/cli/internal/event/application/consume"
)

func sampleView() appconsume.DecisionView {
	return appconsume.DecisionView{
		EventKey: "vc.note.generated_v1",
		Domain:   "vc",
		Identity: "user",
		Status:   "ready",
		Params:   map[string]string{"whiteboard_id": "wb-1", "access_token": "sk-SENSITIVE-VALUE"},
		Scope:    "vc.note.generated_v1",
		Preconditions: []appconsume.PreconditionView{
			{Name: "console_event_published", Status: "ok"},
			{Name: "scopes_granted", Status: "ok"},
		},
		Preparation: &appconsume.PreparationView{
			Strategy: "legacy_preconsume", Condition: "first_consumer_for_scope", Action: "register_event_delivery",
		},
		WouldRead:  []string{"local_bus_probe", "app_metadata_preflight"},
		WouldWrite: []string{"start_or_reuse_local_bus", "register_consumer", "run_preparation_when_first", "open_event_stream"},
	}
}

// The JSON contract: dry_run is the envelope's own top-level marker (never a
// data field), and the decision sits under data.decision with its documented
// members.
func TestWriteDecisionJSON_EnvelopeContract(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := WriteDecisionJSON(&out, &errOut, "user", sampleView()); err != nil {
		t.Fatal(err)
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, out.String())
	}
	if string(envelope["ok"]) != "true" || string(envelope["dry_run"]) != "true" {
		t.Errorf("envelope must carry top-level ok=true and dry_run=true, got %s", out.String())
	}
	if _, misplaced := envelope["decision"]; misplaced {
		t.Error("decision must live under data, not at the envelope top level")
	}

	var data struct {
		Decision struct {
			EventKey    string            `json:"event_key"`
			Domain      string            `json:"domain"`
			Identity    string            `json:"identity"`
			Status      string            `json:"status"`
			Params      map[string]string `json:"params"`
			Scope       string            `json:"scope"`
			Preparation *struct {
				Strategy  string `json:"strategy"`
				Condition string `json:"condition"`
				Action    string `json:"action"`
			} `json:"preparation"`
			WouldRead  []string `json:"would_read"`
			WouldWrite []string `json:"would_write"`
			DryRun     *bool    `json:"dry_run"`
		} `json:"decision"`
	}
	if err := json.Unmarshal(envelope["data"], &data); err != nil {
		t.Fatalf("data.decision does not match the documented shape: %v", err)
	}
	d := data.Decision
	if d.EventKey != "vc.note.generated_v1" || d.Domain != "vc" || d.Identity != "user" || d.Status != "ready" {
		t.Errorf("identity facts drifted: %+v", d)
	}
	if d.Preparation == nil || d.Preparation.Condition != "first_consumer_for_scope" {
		t.Errorf("conditional preparation must be stated: %+v", d.Preparation)
	}
	if len(d.WouldRead) == 0 || len(d.WouldWrite) == 0 {
		t.Error("would_read / would_write must be present")
	}
	if d.DryRun != nil {
		t.Error("dry_run inside data.decision would duplicate the envelope marker")
	}
}

// Sensitive parameter values never reach the rendered output. The control
// assertion first proves the sentinel would be visible if leaked.
func TestWriteDecision_RedactsSensitiveParams(t *testing.T) {
	const sentinel = "sk-SENSITIVE-VALUE"
	view := sampleView()
	if !strings.Contains(view.Params["access_token"], sentinel) {
		t.Fatal("control failed: the sentinel is not in the input, the test cannot prove redaction")
	}

	var jsonOut, jsonErr bytes.Buffer
	if err := WriteDecisionJSON(&jsonOut, &jsonErr, "user", view); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(jsonOut.String(), sentinel) {
		t.Errorf("JSON output leaks a sensitive param value: %s", jsonOut.String())
	}
	compact := strings.ReplaceAll(strings.ReplaceAll(jsonOut.String(), "\n", ""), " ", "")
	if !strings.Contains(compact, `"access_token":"[redacted]"`) {
		t.Errorf("sensitive param must render as redacted, got: %s", jsonOut.String())
	}
	if !strings.Contains(compact, `"whiteboard_id":"wb-1"`) {
		t.Errorf("non-sensitive params must render verbatim, got: %s", jsonOut.String())
	}
}
