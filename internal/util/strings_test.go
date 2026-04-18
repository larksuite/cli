// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package util

import "testing"

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncate", "hello world", 5, "hello"},
		{"empty", "", 5, ""},
		{"zero limit", "hello", 0, ""},
		{"CJK characters", "你好世界测试", 4, "你好世界"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateStr(tt.s, tt.n); got != tt.want {
				t.Errorf("TruncateStr(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestTruncateStrWithEllipsis(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncate with ellipsis", "hello world", 8, "hello..."},
		{"limit less than 3", "hello", 2, "he"},
		{"limit equals 3", "hello world", 3, "..."},
		{"empty", "", 5, ""},
		{"CJK with ellipsis", "你好世界测试", 5, "你好..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateStrWithEllipsis(tt.s, tt.n); got != tt.want {
				t.Errorf("TruncateStrWithEllipsis(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}
