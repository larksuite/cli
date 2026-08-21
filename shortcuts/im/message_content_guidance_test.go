// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func shortcutByCommand(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, sc := range Shortcuts() {
		if sc.Command == command {
			return sc
		}
	}
	t.Fatalf("shortcut %s not registered in Shortcuts()", command)
	return common.Shortcut{}
}

func TestMessageContentGuidance(t *testing.T) {
	for _, command := range []string{"+messages-send", "+messages-reply"} {
		sc := shortcutByCommand(t, command)
		if strings.Contains(strings.ToLower(sc.Description), "text/markdown/post/media") {
			t.Errorf("%s description mixes input formats and msg_type values: %q", command, sc.Description)
		}
		for _, keyword := range []string{"text", "markdown", "post", "media"} {
			if !strings.Contains(strings.ToLower(sc.Description), keyword) {
				t.Errorf("%s description missing routing keyword %q: %q", command, keyword, sc.Description)
			}
		}

		var msgType common.Flag
		for _, flag := range sc.Flags {
			if flag.Name == "msg-type" {
				msgType = flag
				break
			}
		}
		if slices.Contains(msgType.Enum, "markdown") || !slices.Contains(msgType.Enum, "post") {
			t.Errorf("%s --msg-type enum = %v, want post and no markdown", command, msgType.Enum)
		}

		count := 0
		for _, tip := range sc.Tips {
			if tip == messageContentTip {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%s content tip count = %d, want 1", command, count)
		}
	}

	doc, err := os.ReadFile("../../skills/lark-im/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(doc))
	if strings.Contains(text, "text/markdown/post/media") {
		t.Fatal("lark-im skill still mixes content inputs with msg_type values")
	}
	for _, keyword := range []string{"plain text", "markdown input", "post json", "media", "images", "files", "video", "audio"} {
		if !strings.Contains(text, keyword) {
			t.Errorf("lark-im skill missing content routing keyword %q", keyword)
		}
	}
}

func TestMarkdownMsgTypeRecovery(t *testing.T) {
	for _, value := range []string{"markdown", "Markdown"} {
		t.Run(value, func(t *testing.T) {
			cmd := &cobra.Command{Use: "message"}
			cmd.Flags().String("msg-type", "text", "")
			cmd.Flags().String("content", "", "")
			previousCalled := false
			cmd.PreRunE = func(*cobra.Command, []string) error {
				previousCalled = true
				return nil
			}

			chainMarkdownMsgTypeRecovery(cmd)
			if err := cmd.Flags().Set("msg-type", value); err != nil {
				t.Fatal(err)
			}
			err := cmd.PreRunE(cmd, nil)
			if !previousCalled {
				t.Fatal("existing PreRunE was not preserved")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed validation error", err, err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("problem = %#v, want validation/invalid_argument", problem)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if problem.Message != markdownMsgTypeMessage || validationErr.Param != "--msg-type" || problem.Hint != markdownMsgTypeHint {
				t.Fatalf("problem = %#v", problem)
			}
		})
	}
}

func TestMarkdownMsgTypeRecoveryDefersOtherValues(t *testing.T) {
	cmd := &cobra.Command{Use: "message"}
	cmd.Flags().String("msg-type", "text", "")
	chainMarkdownMsgTypeRecovery(cmd)

	for _, value := range []string{"post", "invalid"} {
		if err := cmd.Flags().Set("msg-type", value); err != nil {
			t.Fatal(err)
		}
		if err := cmd.PreRunE(cmd, nil); err != nil {
			t.Fatalf("PreRunE(%q) = %v, want generic runner to decide", value, err)
		}
	}
}

func TestMarkdownMsgTypeRecoveryPreservesFlagReadFailure(t *testing.T) {
	cmd := &cobra.Command{Use: "message"}
	cmd.Flags().Bool("msg-type", false, "")
	chainMarkdownMsgTypeRecovery(cmd)
	if err := cmd.Flags().Set("msg-type", "true"); err != nil {
		t.Fatal(err)
	}

	err := cmd.PreRunE(cmd, nil)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("error = %T %v, want typed internal flag-read error", err, err)
	}
}
