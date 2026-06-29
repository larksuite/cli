// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

func TestRequireConfirmation_TypedShape(t *testing.T) {
	err := RequireConfirmation("drive +delete")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var cre *errs.ConfirmationRequiredError
	if !errors.As(err, &cre) {
		t.Fatalf("expected *errs.ConfirmationRequiredError, got %T", err)
	}
	if cre.Category != errs.CategoryConfirmation {
		t.Errorf("Category = %q, want %q", cre.Category, errs.CategoryConfirmation)
	}
	if cre.Subtype != errs.SubtypeConfirmationRequired {
		t.Errorf("Subtype = %q, want %q", cre.Subtype, errs.SubtypeConfirmationRequired)
	}
	if got := output.ExitCodeOf(err); got != output.ExitConfirmationRequired {
		t.Errorf("ExitCodeOf = %d, want %d", got, output.ExitConfirmationRequired)
	}
	if !strings.Contains(cre.Message, "drive +delete") || !strings.Contains(cre.Message, "requires confirmation") {
		t.Errorf("Message = %q, want it to mention action and 'requires confirmation'", cre.Message)
	}
	if cre.Hint != "add --yes to confirm" {
		t.Errorf("Hint = %q, want 'add --yes to confirm'", cre.Hint)
	}
	if cre.Risk != errs.RiskHighRiskWrite {
		t.Errorf("Risk = %q, want %q", cre.Risk, errs.RiskHighRiskWrite)
	}
	if cre.Action != "drive +delete" {
		t.Errorf("Action = %q, want drive +delete", cre.Action)
	}
}

func TestRequireConfirmation_JSONShape(t *testing.T) {
	err := RequireConfirmation("mail +send")
	var cre *errs.ConfirmationRequiredError
	if !errors.As(err, &cre) {
		t.Fatalf("expected *errs.ConfirmationRequiredError, got %T", err)
	}
	raw, mErr := json.Marshal(cre)
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

	if back["risk"] != "high-risk-write" {
		t.Errorf("risk in JSON = %v", back["risk"])
	}
	if back["action"] != "mail +send" {
		t.Errorf("action in JSON = %v", back["action"])
	}
	// Action-only protocol: no UpgradedBy / fix_command / upgraded_by leak.
	if _, has := back["upgraded_by"]; has {
		t.Errorf("unexpected upgraded_by present in JSON: %s", raw)
	}
}
