// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestRequireConfirmation_EnvelopeShape(t *testing.T) {
	err := RequireConfirmation("drive +delete")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitConfirmationRequired {
		t.Errorf("Code = %d, want %d", exitErr.Code, output.ExitConfirmationRequired)
	}
	if exitErr.Detail == nil {
		t.Fatal("Detail is nil")
	}
	d := exitErr.Detail
	if d.Type != "confirmation_required" {
		t.Errorf("Type = %q, want confirmation_required", d.Type)
	}
	if !strings.Contains(d.Message, "drive +delete") || !strings.Contains(d.Message, "requires confirmation") {
		t.Errorf("Message = %q, want it to mention action and 'requires confirmation'", d.Message)
	}
	if d.Hint != "add --yes to confirm" {
		t.Errorf("Hint = %q, want 'add --yes to confirm'", d.Hint)
	}
	if d.Risk == nil {
		t.Fatal("Risk is nil")
	}
	if d.Risk.Level != "high-risk-write" {
		t.Errorf("Risk.Level = %q, want high-risk-write", d.Risk.Level)
	}
	if d.Risk.Action != "drive +delete" {
		t.Errorf("Risk.Action = %q, want drive +delete", d.Risk.Action)
	}
}

func TestRequireConfirmation_JSONShape(t *testing.T) {
	err := RequireConfirmation("mail +send")
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	raw, mErr := json.Marshal(exitErr.Detail)
	if mErr != nil {
		t.Fatalf("marshal: %v", mErr)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// No fix_command field leaks into the envelope: the protocol avoids
	// shell-quoting hazards by delegating retry to agent-side logic.
	if _, has := back["fix_command"]; has {
		t.Errorf("unexpected fix_command present in JSON: %s", raw)
	}

	risk, ok := back["risk"].(map[string]interface{})
	if !ok {
		t.Fatalf("risk block missing in JSON: %s", raw)
	}
	if risk["level"] != "high-risk-write" {
		t.Errorf("risk.level in JSON = %v", risk["level"])
	}
	if risk["action"] != "mail +send" {
		t.Errorf("risk.action in JSON = %v", risk["action"])
	}
	// Action-only protocol: no UpgradedBy / fix_command / upgraded_by leak.
	if _, has := risk["upgraded_by"]; has {
		t.Errorf("unexpected upgraded_by present in JSON: %s", raw)
	}
}
