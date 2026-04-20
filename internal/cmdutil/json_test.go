// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import "testing"

func TestParseOptionalBody(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOptionalBody(tt.method, tt.data, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOptionalBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && got != nil {
				t.Errorf("ParseOptionalBody() = %v, want nil", got)
			}
			if !tt.wantNil && !tt.wantErr && got == nil {
				t.Error("ParseOptionalBody() = nil, want non-nil")
			}
		})
	}
}

func TestParseJSONMap(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		label   string
		wantLen int
		wantErr bool
	}{
		{"empty input", "", "--params", 0, false},
		{"valid json", `{"a":"1","b":"2"}`, "--params", 2, false},
		{"invalid json", `{bad}`, "--params", 0, true},
		{"json array", `[1,2]`, "--data", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSONMap(tt.input, tt.label, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseJSONMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("ParseJSONMap() returned map with %d keys, want %d", len(got), tt.wantLen)
			}
		})
	}
}
