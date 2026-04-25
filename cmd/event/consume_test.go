// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"strings"
	"testing"
)

func TestParseParams(t *testing.T) {
	cases := []struct {
		name       string
		in         []string
		want       map[string]string
		wantSentry error
		wantEcho   string
	}{
		{
			name: "empty input",
			in:   nil,
			want: map[string]string{},
		},
		{
			name: "single key=value",
			in:   []string{"mailbox=user@example.com"},
			want: map[string]string{"mailbox": "user@example.com"},
		},
		{
			name: "multiple pairs",
			in:   []string{"a=1", "b=2", "c=3"},
			want: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name: "value containing = is kept intact",
			in:   []string{"filter=foo=bar"},
			want: map[string]string{"filter": "foo=bar"},
		},
		{
			name: "empty value allowed",
			in:   []string{"key="},
			want: map[string]string{"key": ""},
		},
		{
			name: "duplicate key — last wins",
			in:   []string{"k=1", "k=2"},
			want: map[string]string{"k": "2"},
		},
		{
			name:       "missing = separator",
			in:         []string{"mailbox"},
			wantSentry: errInvalidParamFormat,
			wantEcho:   `"mailbox"`,
		},
		{
			name:       "leading = (empty key)",
			in:         []string{"=value"},
			wantSentry: errInvalidParamFormat,
			wantEcho:   `"=value"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseParams(tc.in)
			if tc.wantSentry != nil {
				if err == nil {
					t.Fatalf("want error wrapping %v, got nil", tc.wantSentry)
				}
				if !errors.Is(err, tc.wantSentry) {
					t.Fatalf("want errors.Is(err, %v), got %q", tc.wantSentry, err.Error())
				}
				if tc.wantEcho != "" && !strings.Contains(err.Error(), tc.wantEcho) {
					t.Errorf("err %q should echo %q so user sees the bad input", err.Error(), tc.wantEcho)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got=%v", len(got), len(tc.want), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestSanitizeOutputDir(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantSentry error
	}{
		{
			name: "relative path accepted",
			in:   "./output",
		},
		{
			name: "nested relative path accepted",
			in:   "events/today",
		},
		{
			name:       "tilde rejected explicitly",
			in:         "~/events",
			wantSentry: errOutputDirTilde,
		},
		{
			name:       "parent escape rejected",
			in:         "../outside",
			wantSentry: errOutputDirUnsafe,
		},
		{
			name:       "absolute path rejected",
			in:         "/tmp/events",
			wantSentry: errOutputDirUnsafe,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sanitizeOutputDir(tc.in)
			if tc.wantSentry != nil {
				if err == nil {
					t.Fatalf("want error wrapping %v, got nil (path=%q)", tc.wantSentry, got)
				}
				if !errors.Is(err, tc.wantSentry) {
					t.Fatalf("want errors.Is(err, %v), got %q", tc.wantSentry, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == "" {
				t.Errorf("expected non-empty safe path, got %q", got)
			}
		})
	}
}
