// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"testing"
)

func TestModeFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		want     mode
		wantWarn bool
	}{
		{"empty", "", modeOff, false},
		{"off", "off", modeOff, false},
		{"OFF", "OFF", modeOff, false},
		{"warn", "warn", modeWarn, false},
		{"WARN", "WARN", modeWarn, false},
		{"block", "block", modeBlock, false},
		{"unknown", "banana", modeOff, true},
		{"whitespace", "  warn  ", modeWarn, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", tt.envVal)
			var buf bytes.Buffer
			got := modeFromEnv(&buf)
			if got != tt.want {
				t.Errorf("modeFromEnv() = %d, want %d", got, tt.want)
			}
			if tt.wantWarn && buf.Len() == 0 {
				t.Error("expected stderr warning")
			}
			if !tt.wantWarn && buf.Len() > 0 {
				t.Errorf("unexpected stderr: %s", buf.String())
			}
		})
	}
}

func TestNormalizeCommandPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"lark-cli im +messages-search", "im.messages_search"},
		{"lark-cli drive upload +file", "drive.upload.file"},
		{"lark-cli api GET /path", "api.GET./path"},
		{"lark-cli", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeCommandPath(tt.input)
			if got != tt.want {
				t.Errorf("normalizeCommandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
