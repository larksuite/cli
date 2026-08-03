// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestBuildAPIError_CategoryConfirmationFillsRiskAction pins fail-closed
// behaviour: a code mapped to CategoryConfirmation MUST yield a
// ConfirmationRequiredError whose Risk + Action are non-empty even when the
// CodeMeta itself carries no Risk/Action hints. Risk falls back to
// RiskUnknown; Action falls back to ctx.LarkCmd.
func TestBuildAPIError_CategoryConfirmationFillsRiskAction(t *testing.T) {
	const stubCode = 99999991
	codeMeta[stubCode] = CodeMeta{
		Category: errs.CategoryConfirmation,
		Subtype:  errs.SubtypeConfirmationRequired,
	}
	t.Cleanup(func() { delete(codeMeta, stubCode) })

	resp := map[string]any{"code": stubCode, "msg": "confirmation required"}
	ctx := ClassifyContext{
		Brand:    "feishu",
		AppID:    "cli_test",
		Identity: "user",
		LarkCmd:  "drive +delete",
	}
	err := BuildAPIError(resp, ctx)
	var confirmErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmErr) {
		t.Fatalf("expected *ConfirmationRequiredError, got %T: %v", err, err)
	}
	if confirmErr.Risk == "" {
		t.Error("Risk empty; arm must fail-closed with RiskUnknown")
	}
	if confirmErr.Risk != errs.RiskUnknown {
		t.Errorf("Risk = %q, want %q (CodeMeta carried no Risk hint)",
			confirmErr.Risk, errs.RiskUnknown)
	}
	if confirmErr.Action == "" {
		t.Error("Action empty; arm must fail-closed with command name from ClassifyContext")
	}
	if confirmErr.Action != "drive +delete" {
		t.Errorf("Action = %q, want %q (ctx.LarkCmd fallback)",
			confirmErr.Action, "drive +delete")
	}
}

// TestBuildAPIError_CategoryConfirmationPrefersCodeMetaHints pins that when
// CodeMeta carries explicit Risk + Action, the dispatcher uses them rather
// than falling back to RiskUnknown / ctx.LarkCmd.
func TestBuildAPIError_CategoryConfirmationPrefersCodeMetaHints(t *testing.T) {
	const stubCode = 99999992
	codeMeta[stubCode] = CodeMeta{
		Category: errs.CategoryConfirmation,
		Subtype:  errs.SubtypeConfirmationRequired,
		Risk:     errs.RiskHighRiskWrite,
		Action:   "wiki:delete-space",
	}
	t.Cleanup(func() { delete(codeMeta, stubCode) })

	resp := map[string]any{"code": stubCode, "msg": "confirmation required"}
	ctx := ClassifyContext{LarkCmd: "drive +delete"}
	err := BuildAPIError(resp, ctx)
	var confirmErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmErr) {
		t.Fatalf("expected *ConfirmationRequiredError, got %T: %v", err, err)
	}
	if confirmErr.Risk != errs.RiskHighRiskWrite {
		t.Errorf("Risk = %q, want %q (CodeMeta hint should win)",
			confirmErr.Risk, errs.RiskHighRiskWrite)
	}
	if confirmErr.Action != "wiki:delete-space" {
		t.Errorf("Action = %q, want %q (CodeMeta hint should win)",
			confirmErr.Action, "wiki:delete-space")
	}
}

// TestBuildAPIError_UnknownCategoryRoutesToInternalError pins fail-closed
// behaviour: an unrecognized Category routes to InternalError instead of
// emitting an empty Problem on the wire.
func TestBuildAPIError_UnknownCategoryRoutesToInternalError(t *testing.T) {
	const stubCode = 99999993
	codeMeta[stubCode] = CodeMeta{
		Category: errs.Category("totally_unknown_category"),
		Subtype:  errs.SubtypeUnknown,
	}
	t.Cleanup(func() { delete(codeMeta, stubCode) })

	resp := map[string]any{"code": stubCode, "msg": "weird"}
	err := BuildAPIError(resp, ClassifyContext{})
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("expected *InternalError, got %T: %v", err, err)
	}
	if ie.Category != errs.CategoryInternal {
		t.Errorf("Category = %q, want %q", ie.Category, errs.CategoryInternal)
	}
	if ie.Subtype != errs.SubtypeSDKError {
		t.Errorf("Subtype = %q, want %q", ie.Subtype, errs.SubtypeSDKError)
	}
	if ie.Code != stubCode {
		t.Errorf("Code = %d, want %d (raw Lark code should propagate)", ie.Code, stubCode)
	}
}

// TestBuildAPIError_ConfigInvalidClient_HasHint pins that when a
// CategoryConfig response (Lark code 10014 — "app secret invalid") flows
// through BuildAPIError, the resulting *ConfigError MUST carry the canonical
// recovery hint pointing the user at `lark-cli config set-app-secret --app-secret-stdin`
// (secret rotation, not profile re-init).
func TestBuildAPIError_ConfigInvalidClient_HasHint(t *testing.T) {
	const code = 10014
	resp := map[string]any{"code": code, "msg": "app secret invalid"}
	ctx := ClassifyContext{Brand: "feishu", AppID: "cli_test", Identity: "bot"}

	err := BuildAPIError(resp, ctx)
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidClient {
		t.Errorf("Subtype = %q, want %q", cfgErr.Subtype, errs.SubtypeInvalidClient)
	}
	if cfgErr.Hint == "" {
		t.Errorf("Hint is empty; canonical hint required for invalid_client")
	}
	if !strings.Contains(cfgErr.Hint, "config set-app-secret --app-secret-stdin") {
		t.Errorf("Hint should reference `config set-app-secret --app-secret-stdin`; got %q", cfgErr.Hint)
	}
	if strings.Contains(cfgErr.Hint, "config init") {
		t.Errorf("Hint must not point to config init; got %q", cfgErr.Hint)
	}
}
