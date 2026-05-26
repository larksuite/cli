// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestWriteErrorEnvelope_WithNotice(t *testing.T) {
	// Set up PendingNotice
	origNotice := PendingNotice
	PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{
			"update": map[string]interface{}{
				"current": "1.0.0",
				"latest":  "2.0.0",
			},
		}
	}
	defer func() { PendingNotice = origNotice }()

	exitErr := &ExitError{
		Code:   1,
		Detail: &ErrDetail{Type: "api_error", Message: "something failed"},
	}

	var buf bytes.Buffer
	WriteErrorEnvelope(&buf, exitErr, "user")

	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	// Verify _notice is present
	notice, ok := env["_notice"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _notice field in output")
	}
	update, ok := notice["update"].(map[string]interface{})
	if !ok {
		t.Fatal("expected _notice.update field")
	}
	if update["latest"] != "2.0.0" {
		t.Errorf("expected latest=2.0.0, got %v", update["latest"])
	}

	// Verify standard fields
	if env["ok"] != false {
		t.Error("expected ok=false")
	}
	if env["identity"] != "user" {
		t.Errorf("expected identity=user, got %v", env["identity"])
	}
}

func TestWriteErrorEnvelope_WithoutNotice(t *testing.T) {
	// Ensure PendingNotice is nil
	origNotice := PendingNotice
	PendingNotice = nil
	defer func() { PendingNotice = origNotice }()

	exitErr := &ExitError{
		Code:   1,
		Detail: &ErrDetail{Type: "api_error", Message: "something failed"},
	}

	var buf bytes.Buffer
	WriteErrorEnvelope(&buf, exitErr, "bot")

	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if _, ok := env["_notice"]; ok {
		t.Error("expected no _notice field when PendingNotice is nil")
	}
}

func TestWriteErrorEnvelope_NilDetail(t *testing.T) {
	exitErr := &ExitError{Code: 1}

	var buf bytes.Buffer
	WriteErrorEnvelope(&buf, exitErr, "user")

	if buf.Len() != 0 {
		t.Errorf("expected no output for nil Detail, got: %s", buf.String())
	}
}

func TestGetNotice(t *testing.T) {
	// Nil PendingNotice → nil
	origNotice := PendingNotice
	PendingNotice = nil
	if got := GetNotice(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	// With PendingNotice → returns value
	PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"update": "test"}
	}
	got := GetNotice()
	if got == nil || got["update"] != "test" {
		t.Errorf("expected {update: test}, got %v", got)
	}

	// PendingNotice returns nil → nil
	PendingNotice = func() map[string]interface{} { return nil }
	if got := GetNotice(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	PendingNotice = origNotice
}

// TestErrValidation_LegacyExitErrorShape pins the stage-1 wire contract for
// output.ErrValidation: the helper MUST return *output.ExitError (so callers
// using errors.As(&exitErr) continue to work), with wire fields restricted
// to type+message — no `subtype` emission. The typed envelope shape (which
// adds subtype, param, etc.) is reserved for stage-2 per-domain migration.
func TestErrValidation_LegacyExitErrorShape(t *testing.T) {
	err := ErrValidation("bad arg: %s", "x")

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("ErrValidation must return *ExitError, got %T", err)
	}
	if exitErr.Code != ExitValidation {
		t.Errorf("Code = %d, want ExitValidation (%d)", exitErr.Code, ExitValidation)
	}
	if exitErr.Detail == nil {
		t.Fatal("Detail must be populated")
	}
	if exitErr.Detail.Type != "validation" {
		t.Errorf("Detail.Type = %q, want %q", exitErr.Detail.Type, "validation")
	}
	if exitErr.Detail.Message != "bad arg: x" {
		t.Errorf("Detail.Message = %q, want %q", exitErr.Detail.Message, "bad arg: x")
	}

	// Wire envelope must have only type+message — no subtype field.
	var buf bytes.Buffer
	WriteErrorEnvelope(&buf, exitErr, "user")
	var wire map[string]any
	if err := json.Unmarshal(buf.Bytes(), &wire); err != nil {
		t.Fatalf("envelope JSON parse failed: %v\nraw: %s", err, buf.String())
	}
	errObj, ok := wire["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'error' object; got: %s", buf.String())
	}
	if _, hasSubtype := errObj["subtype"]; hasSubtype {
		t.Errorf("legacy ErrValidation envelope must NOT emit `subtype`; got: %s", buf.String())
	}
	if errObj["type"] != "validation" {
		t.Errorf("envelope error.type = %v, want \"validation\"", errObj["type"])
	}
}

// TestErrNetwork_LegacyExitErrorShape pins the stage-1 wire contract for
// output.ErrNetwork: same legacy *output.ExitError shape as ErrValidation —
// no subtype field, errors.As(&exitErr) must succeed, exit code ExitNetwork.
func TestErrNetwork_LegacyExitErrorShape(t *testing.T) {
	err := ErrNetwork("conn refused: %s", "10.0.0.1")

	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("ErrNetwork must return *ExitError, got %T", err)
	}
	if exitErr.Code != ExitNetwork {
		t.Errorf("Code = %d, want ExitNetwork (%d)", exitErr.Code, ExitNetwork)
	}
	if exitErr.Detail == nil {
		t.Fatal("Detail must be populated")
	}
	if exitErr.Detail.Type != "network" {
		t.Errorf("Detail.Type = %q, want %q", exitErr.Detail.Type, "network")
	}
	if exitErr.Detail.Message != "conn refused: 10.0.0.1" {
		t.Errorf("Detail.Message = %q, want %q", exitErr.Detail.Message, "conn refused: 10.0.0.1")
	}

	// Wire envelope must have only type+message — no subtype field.
	var buf bytes.Buffer
	WriteErrorEnvelope(&buf, exitErr, "user")
	var wire map[string]any
	if err := json.Unmarshal(buf.Bytes(), &wire); err != nil {
		t.Fatalf("envelope JSON parse failed: %v\nraw: %s", err, buf.String())
	}
	errObj, ok := wire["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope missing 'error' object; got: %s", buf.String())
	}
	if _, hasSubtype := errObj["subtype"]; hasSubtype {
		t.Errorf("legacy ErrNetwork envelope must NOT emit `subtype`; got: %s", buf.String())
	}
	if errObj["type"] != "network" {
		t.Errorf("envelope error.type = %v, want \"network\"", errObj["type"])
	}
}
