// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

// requireJSONInputValidationError asserts err is a typed *errs.ValidationError
// with subtype invalid_argument, exit code 2 (legacy ErrValidation parity),
// and the offending flag recorded as Param.
func requireJSONInputValidationError(t *testing.T, err error, wantParam string) {
	t.Helper()
	var valErr *errs.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if valErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", valErr.Subtype, errs.SubtypeInvalidArgument)
	}
	if valErr.Param != wantParam {
		t.Errorf("param = %q, want %q", valErr.Param, wantParam)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d (ExitValidation, legacy parity)", got, output.ExitValidation)
	}
	if valErr.Cause == nil {
		t.Error("expected the underlying parse/resolve error attached as Cause")
	}
}

func TestParseOptionalBody(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	tests := []struct {
		name    string
		method  string
		data    string
		wantNil bool
		wantErr bool
	}{
		{"GET ignored", "GET", `{"a":1}`, true, false},
		{"POST empty data", "POST", "", true, false},
		{"POST valid", "POST", `{"key":"val"}`, false, false},
		{"PUT valid", "PUT", `[1,2,3]`, false, false},
		{"PATCH valid", "PATCH", `"hello"`, false, false},
		{"DELETE valid", "DELETE", `{"id":"1"}`, false, false},
		{"POST invalid json", "POST", `{bad}`, true, true},
		{"POST unreadable @file", "POST", "@/nonexistent/body.json", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOptionalBody(tt.method, tt.data, nil, fio)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOptionalBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				requireJSONInputValidationError(t, err, "--data")
				return
			}
			if tt.wantNil && got != nil {
				t.Errorf("ParseOptionalBody() = %v, want nil", got)
			}
			if !tt.wantNil && got == nil {
				t.Error("ParseOptionalBody() = nil, want non-nil")
			}
		})
	}
}

func TestParseJSONMap(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	tests := []struct {
		name    string
		input   string
		label   string
		wantLen int
		wantErr bool
	}{
		{"empty input", "", "--params", 0, false},
		{"json null", "null", "--params", 0, false},
		{"valid json", `{"a":"1","b":"2"}`, "--params", 2, false},
		{"invalid json", `{bad}`, "--params", 0, true},
		{"json array", `[1,2]`, "--data", 0, true},
		{"unreadable @file", "@/nonexistent/params.json", "--params", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSONMap(tt.input, tt.label, nil, fio)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseJSONMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				requireJSONInputValidationError(t, err, tt.label)
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("ParseJSONMap() returned map with %d keys, want %d", len(got), tt.wantLen)
			}
			// A successful parse must yield a non-nil, writable map: callers
			// overlay onto it (params[k]=v), so `null` — which unmarshals to a
			// nil map without error — must normalize to {} like empty input.
			if !tt.wantErr && got == nil {
				t.Error("ParseJSONMap() = nil map on success, want non-nil")
			}
		})
	}
}
