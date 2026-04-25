// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"strings"
	"testing"

	_ "github.com/larksuite/cli/events"
)

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "abc", 3},
		{"kitten", "kitten", 0},
		{"kitten", "sitten", 1},
		{"kitten", "sitting", 3},
		{"飞书", "飞书", 0},
		{"飞书", "飞s", 1},
	}
	for _, tc := range cases {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSuggestEventKeys(t *testing.T) {
	cases := []struct {
		name              string
		input             string
		wantEmpty         bool
		wantAllHavePrefix string
		wantContains      string
	}{
		{
			name:         "typo via Levenshtein (recieve → receive)",
			input:        "im.message.recieve_v1",
			wantContains: "im.message.receive_v1",
		},
		{
			name:              "substring match returns im.message.* keys",
			input:             "im.message",
			wantAllHavePrefix: "im.message.",
		},
		{
			name:      "completely unrelated input returns empty",
			input:     "xyzzy_no_such_event_key_at_all",
			wantEmpty: true,
		},
		{
			name:         "exact key is a substring of itself",
			input:        "im.message.receive_v1",
			wantContains: "im.message.receive_v1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := suggestEventKeys(tc.input)
			if tc.wantEmpty {
				if len(got) != 0 {
					t.Errorf("expected empty slice, got %v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("expected non-empty suggestions, got nothing")
			}
			if len(got) > maxSuggestions {
				t.Errorf("got %d suggestions, want at most %d: %v", len(got), maxSuggestions, got)
			}
			if tc.wantAllHavePrefix != "" {
				for _, k := range got {
					if !strings.HasPrefix(k, tc.wantAllHavePrefix) {
						t.Errorf("suggestion %q lacks prefix %q (full slice: %v)", k, tc.wantAllHavePrefix, got)
					}
				}
			}
			if tc.wantContains != "" {
				found := false
				for _, k := range got {
					if k == tc.wantContains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("want %q in suggestions, got %v", tc.wantContains, got)
				}
			}
		})
	}
}

func TestFormatSuggestions(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{name: "empty → empty string", in: nil, want: ""},
		{name: "single key → just quoted", in: []string{"a"}, want: `"a"`},
		{name: "two keys → one of", in: []string{"a", "b"}, want: `one of: "a", "b"`},
		{name: "three keys → one of", in: []string{"a", "b", "c"}, want: `one of: "a", "b", "c"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatSuggestions(tc.in); got != tc.want {
				t.Errorf("formatSuggestions(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestUnknownEventKeyErr_IncludesSuggestion(t *testing.T) {
	err := unknownEventKeyErr("im.message.recieve_v1")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{
		"unknown EventKey: im.message.recieve_v1",
		"did you mean",
		"im.message.receive_v1",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestUnknownEventKeyErr_NoSuggestion(t *testing.T) {
	err := unknownEventKeyErr("xyzzy_no_such_event_key_at_all")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown EventKey") {
		t.Errorf("error should mention unknown EventKey: %q", msg)
	}
	if strings.Contains(msg, "did you mean") {
		t.Errorf("error should NOT suggest anything for nonsense input: %q", msg)
	}
}
