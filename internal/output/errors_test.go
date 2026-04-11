// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestMarkRaw_ExitError(t *testing.T) {
	err := ErrAPI(99991672, "API error: [99991672] scope not enabled", nil)
	if err.Raw {
		t.Fatal("expected Raw=false before MarkRaw")
	}

	result := MarkRaw(err)
	if result != err {
		t.Error("expected MarkRaw to return the same error")
	}
	if !err.Raw {
		t.Error("expected Raw=true after MarkRaw")
	}
}

func TestMarkRaw_NonExitError(t *testing.T) {
	plain := fmt.Errorf("some plain error")
	result := MarkRaw(plain)
	if result != plain {
		t.Error("expected MarkRaw to return the same error for non-ExitError")
	}
}

func TestMarkRaw_Nil(t *testing.T) {
	result := MarkRaw(nil)
	if result != nil {
		t.Error("expected MarkRaw(nil) to return nil")
	}
}

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
