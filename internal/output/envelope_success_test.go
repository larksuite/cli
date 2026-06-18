// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
)

func TestSuccessEnvelopeData_ExtractsBusinessData(t *testing.T) {
	result := map[string]interface{}{
		"code": float64(0),
		"msg":  "ok",
		"data": map[string]interface{}{"id": "1"},
	}

	got := SuccessEnvelopeData(result)
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("business data type = %T, want map", got)
	}
	if m["id"] != "1" {
		t.Fatalf("id = %v, want 1", m["id"])
	}
	if _, ok := m["code"]; ok {
		t.Fatal("business data must not contain outer code")
	}
}

func TestSuccessEnvelopeData_MissingDataUsesEmptyObject(t *testing.T) {
	got := SuccessEnvelopeData(map[string]interface{}{"code": float64(0), "msg": "ok"})
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("business data type = %T, want map", got)
	}
	if len(m) != 0 {
		t.Fatalf("business data = %#v, want empty object", m)
	}
}

func TestSuccessEnvelopeData_NilDataUsesEmptyObject(t *testing.T) {
	got := SuccessEnvelopeData(map[string]interface{}{"code": float64(0), "msg": "ok", "data": nil})
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("business data type = %T, want map", got)
	}
	if len(m) != 0 {
		t.Fatalf("business data = %#v, want empty object", m)
	}
}

func TestWriteSuccessEnvelope_PrintsShortcutCompatibleEnvelope(t *testing.T) {
	var out strings.Builder

	err := WriteSuccessEnvelope(map[string]interface{}{"id": "1"}, SuccessEnvelopeOptions{
		Identity: "bot",
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("WriteSuccessEnvelope() error = %v", err)
	}

	var env map[string]interface{}
	if err := json.Unmarshal([]byte(out.String()), &env); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if env["ok"] != true || env["identity"] != "bot" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok || data["id"] != "1" {
		t.Fatalf("unexpected data payload: %#v", env["data"])
	}
	if _, ok := env["code"]; ok {
		t.Fatalf("output leaked protocol field code: %#v", env)
	}
	if _, ok := env["msg"]; ok {
		t.Fatalf("output leaked protocol field msg: %#v", env)
	}
	if _, ok := env["_content_safety_alert"]; ok {
		t.Fatalf("output should omit empty content-safety alert: %#v", env)
	}
}

func TestWriteSuccessEnvelope_JqUsesEnvelope(t *testing.T) {
	var out strings.Builder

	err := WriteSuccessEnvelope(map[string]interface{}{"id": "1"}, SuccessEnvelopeOptions{
		Identity: "bot",
		JqExpr:   ".data.id",
		Out:      &out,
	})
	if err != nil {
		t.Fatalf("WriteSuccessEnvelope() error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "1" {
		t.Fatalf("jq output = %q, want %q", out.String(), "1")
	}
}

func TestWriteSuccessEnvelope_JqWarnsWhenSafetyAlertFiltered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	extcs.Register(&mockProvider{
		name:  "mock",
		alert: &extcs.Alert{Provider: "mock", MatchedRules: []string{"r1"}},
	})
	t.Cleanup(func() { extcs.Register(nil) })

	var out strings.Builder
	var errOut strings.Builder
	err := WriteSuccessEnvelope(map[string]interface{}{"id": "1"}, SuccessEnvelopeOptions{
		CommandPath: "lark-cli im +test",
		Identity:    "bot",
		JqExpr:      ".data.id",
		Out:         &out,
		ErrOut:      &errOut,
	})
	if err != nil {
		t.Fatalf("WriteSuccessEnvelope() error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "1" {
		t.Fatalf("jq output = %q, want %q", out.String(), "1")
	}
	if !strings.Contains(errOut.String(), "warning: content safety alert from mock") {
		t.Fatalf("expected content safety warning on stderr, got: %s", errOut.String())
	}
	if !strings.Contains(errOut.String(), "r1") {
		t.Fatalf("expected rule in stderr warning, got: %s", errOut.String())
	}
}

func TestWriteSuccessEnvelope_BlockModeReturnsTypedErrorWithoutStdout(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	extcs.Register(&mockProvider{
		name:  "mock",
		alert: &extcs.Alert{Provider: "mock", MatchedRules: []string{"r1"}},
	})
	t.Cleanup(func() { extcs.Register(nil) })

	var out strings.Builder
	var errOut strings.Builder
	err := WriteSuccessEnvelope(map[string]interface{}{"id": "1"}, SuccessEnvelopeOptions{
		CommandPath: "lark-cli im +test",
		Identity:    "bot",
		Out:         &out,
		ErrOut:      &errOut,
	})
	if err == nil {
		t.Fatal("expected content safety block error")
	}
	var safetyErr *errs.ContentSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected ContentSafetyError, got %T: %v", err, err)
	}
	if safetyErr.Category != errs.CategoryPolicy || safetyErr.Subtype != errs.SubtypeContentSafety {
		t.Fatalf("problem = %s/%s, want %s/%s", safetyErr.Category, safetyErr.Subtype, errs.CategoryPolicy, errs.SubtypeContentSafety)
	}
	if len(safetyErr.Rules) != 1 || safetyErr.Rules[0] != "r1" {
		t.Fatalf("rules = %v, want [r1]", safetyErr.Rules)
	}
	if !errors.Is(err, errBlocked) {
		t.Fatal("content safety error should preserve errBlocked cause")
	}
	if out.String() != "" {
		t.Fatalf("stdout should stay empty on block, got: %s", out.String())
	}
}
