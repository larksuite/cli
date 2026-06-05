// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"reflect"
	"strings"
	"testing"
)

// ── V2 tests ──

func TestValidCommandsV2(t *testing.T) {
	expected := map[string]bool{
		"str_replace":             true,
		"block_delete":            true,
		"block_insert_after":      true,
		"block_copy_insert_after": true,
		"block_replace":           true,
		"block_move_after":        true,
		"overwrite":               true,
		"append":                  true,
	}
	if len(validCommandsV2) != len(expected) {
		t.Fatalf("expected %d commands, got %d", len(expected), len(validCommandsV2))
	}
	for cmd := range validCommandsV2 {
		if !expected[cmd] {
			t.Fatalf("unexpected command %q in validCommandsV2", cmd)
		}
	}
}

// ── V1 tests ──

func TestSelectionRequiredMessageV1ReplaceAllSuggestsOverwrite(t *testing.T) {
	t.Parallel()

	msg := selectionRequiredMessageV1("replace_all")
	for _, needle := range []string{
		"--replace_all mode requires --selection-with-ellipsis or --selection-by-title",
		"replace the entire document body",
		"--mode overwrite",
	} {
		if !strings.Contains(msg, needle) {
			t.Fatalf("message missing %q: %s", needle, msg)
		}
	}
}

func TestSelectionRequiredMessageV1OtherModesDoNotSuggestOverwrite(t *testing.T) {
	t.Parallel()

	msg := selectionRequiredMessageV1("replace_range")
	if strings.Contains(msg, "--mode overwrite") {
		t.Fatalf("replace_range message should not suggest overwrite: %s", msg)
	}
	if !strings.Contains(msg, "--replace_range mode requires --selection-with-ellipsis or --selection-by-title") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestIsWhiteboardCreateMarkdown(t *testing.T) {
	t.Run("blank whiteboard tags", func(t *testing.T) {
		markdown := "<whiteboard type=\"blank\"></whiteboard>\n<whiteboard type=\"blank\"></whiteboard>"
		if !isWhiteboardCreateMarkdown(markdown) {
			t.Fatalf("expected blank whiteboard markdown to be treated as whiteboard creation")
		}
	})

	t.Run("mermaid code block", func(t *testing.T) {
		markdown := "```mermaid\ngraph TD\nA-->B\n```"
		if !isWhiteboardCreateMarkdown(markdown) {
			t.Fatalf("expected mermaid markdown to be treated as whiteboard creation")
		}
	})

	t.Run("plain markdown", func(t *testing.T) {
		markdown := "## plain text"
		if isWhiteboardCreateMarkdown(markdown) {
			t.Fatalf("did not expect plain markdown to be treated as whiteboard creation")
		}
	})
}

func TestCheckOverwriteResourceBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		markdown string
		wantWarn bool
		wantSubs []string
	}{
		{
			name:     "empty markdown is clean",
			markdown: "",
			wantWarn: false,
		},
		{
			name:     "plain prose is clean",
			markdown: "## Heading\n\nsome text",
			wantWarn: false,
		},
		{
			name:     "single whiteboard triggers warning",
			markdown: `<whiteboard token="abc123"/>`,
			wantWarn: true,
			wantSubs: []string{"1 whiteboard block", "overwrite"},
		},
		{
			name:     "multiple whiteboards counted",
			markdown: "<whiteboard token=\"a\"/>\n<whiteboard token=\"b\"/>",
			wantWarn: true,
			wantSubs: []string{"2 whiteboard blocks"},
		},
		{
			name:     "single file attachment triggers warning",
			markdown: `<file token="tok" name="report.pdf"/>`,
			wantWarn: true,
			wantSubs: []string{"1 file attachment block"},
		},
		{
			name:     "multiple file attachments counted",
			markdown: "<file token=\"a\"/>\n<file token=\"b\"/>\n<file token=\"c\"/>",
			wantWarn: true,
			wantSubs: []string{"3 file attachment blocks"},
		},
		{
			name:     "whiteboard and file together both counted",
			markdown: "<whiteboard token=\"wb\"/>\n<file token=\"f\"/>",
			wantWarn: true,
			wantSubs: []string{"1 whiteboard block", "1 file attachment block"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkOverwriteResourceBlocks(tt.markdown)
			if (got != "") != tt.wantWarn {
				t.Fatalf("checkOverwriteResourceBlocks(%q) = %q, wantWarn=%v", tt.markdown, got, tt.wantWarn)
			}
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("expected warning to contain %q, got: %s", sub, got)
				}
			}
		})
	}
}

func TestNormalizeWhiteboardResult(t *testing.T) {
	t.Run("adds empty board_tokens when whiteboard creation response omits it", func(t *testing.T) {
		result := map[string]interface{}{
			"success": true,
		}

		normalizeWhiteboardResult(result, "<whiteboard type=\"blank\"></whiteboard>")

		got, ok := result["board_tokens"].([]string)
		if !ok {
			t.Fatalf("expected board_tokens to be []string, got %T", result["board_tokens"])
		}
		if len(got) != 0 {
			t.Fatalf("expected empty board_tokens, got %#v", got)
		}
	})

	t.Run("normalizes board_tokens to string slice", func(t *testing.T) {
		result := map[string]interface{}{
			"board_tokens": []interface{}{"board_1", "board_2"},
		}

		normalizeWhiteboardResult(result, "<whiteboard type=\"blank\"></whiteboard>")

		want := []string{"board_1", "board_2"}
		got, ok := result["board_tokens"].([]string)
		if !ok {
			t.Fatalf("expected board_tokens to be []string, got %T", result["board_tokens"])
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("board_tokens mismatch: got %#v want %#v", got, want)
		}
	})

	t.Run("leaves non whiteboard response unchanged", func(t *testing.T) {
		result := map[string]interface{}{
			"success": true,
		}

		normalizeWhiteboardResult(result, "## plain text")

		if _, ok := result["board_tokens"]; ok {
			t.Fatalf("did not expect board_tokens for non-whiteboard markdown")
		}
	})
}

func TestValidateSelectionByTitleV1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		title   string
		wantErr bool
		errSub  string
	}{
		{name: "empty title is valid", title: "", wantErr: false},
		{name: "single heading is valid", title: "## Section", wantErr: false},
		{name: "h1 heading is valid", title: "# Top", wantErr: false},
		{name: "deep heading is valid", title: "### Sub-section", wantErr: false},
		{name: "missing hash prefix is invalid", title: "No hash", wantErr: true, errSub: "'#'"},
		{name: "multiline title is invalid", title: "## First\n## Second", wantErr: true, errSub: "single"},
		{name: "title with embedded carriage return is invalid", title: "## Title\r## Next", wantErr: true, errSub: "single"},
		{name: "leading-space heading is valid after trim", title: "  ## Section", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSelectionByTitleV1(tt.title)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSelectionByTitleV1(%q) error = %v, wantErr = %v", tt.title, err, tt.wantErr)
			}
			if tt.wantErr && tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("expected error to contain %q, got: %v", tt.errSub, err)
			}
		})
	}
}
