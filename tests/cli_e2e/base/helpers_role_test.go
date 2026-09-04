// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "testing"

func TestExtractCreatedRoleID(t *testing.T) {
	tests := []struct {
		name   string
		stdout string
		want   string
	}{
		{
			name:   "direct payload",
			stdout: `{"ok":true,"data":{"role_id":"rol_direct"}}`,
			want:   "rol_direct",
		},
		{
			name:   "nested payload",
			stdout: `{"ok":true,"data":{"data":{"role_id":"rol_nested"}}}`,
			want:   "rol_nested",
		},
		{
			name:   "string encoded nested payload",
			stdout: `{"ok":true,"data":{"data":"{\"role_id\":\"rol_string\"}"}}`,
			want:   "rol_string",
		},
		{
			name:   "string encoded business envelope",
			stdout: `{"ok":true,"data":{"data":"{\"data\":{\"role_id\":\"rol_nested_string\"}}"}}`,
			want:   "rol_nested_string",
		},
		{
			name:   "business envelope",
			stdout: `{"ok":true,"data":{"data":{"data":{"role_id":"rol_business"}}}}`,
			want:   "rol_business",
		},
		{
			name:   "missing",
			stdout: `{"ok":true,"data":{"data":{}}}`,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractCreatedRoleID(tt.stdout); got != tt.want {
				t.Fatalf("extractCreatedRoleID() = %q, want %q", got, tt.want)
			}
		})
	}
}
