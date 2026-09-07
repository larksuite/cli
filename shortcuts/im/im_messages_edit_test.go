// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newEditTestRuntimeContext builds a RuntimeContext wired with the +messages-edit
// flag surface, including the repeatable --set-attachments string-slice flag.
func newEditTestRuntimeContext(stringFlags map[string]string, sliceFlags map[string][]string) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("msg-type", "text", "")
	cmd.Flags().String("content", "", "")
	cmd.Flags().String("text", "", "")
	cmd.Flags().String("markdown", "", "")
	cmd.Flags().StringSlice("set-attachments", nil, "")
	cmd.Flags().Bool("clear-attachments", false, "")
	for name, val := range stringFlags {
		_ = cmd.Flags().Set(name, val)
	}
	for name, vals := range sliceFlags {
		_ = cmd.Flags().Set(name, strings.Join(vals, ","))
	}
	return &common.RuntimeContext{Cmd: cmd}
}

// TestValidateEditContentFlags covers mutual exclusivity of text/markdown/content and the attachment/clear-as-content-source rule.
func TestValidateEditContentFlags(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		markdown         string
		content          string
		attachments      []attachmentItem
		clearAttachments bool
		wantErr          string
	}{
		{name: "text ok", text: "hi", wantErr: ""},
		{name: "markdown ok", markdown: "# hi", wantErr: ""},
		{name: "content ok", content: `{"text":"hi"}`, wantErr: ""},
		{name: "attachment only ok", attachments: []attachmentItem{{Key: "file_1"}}, wantErr: ""},
		{name: "clear only ok", clearAttachments: true, wantErr: ""},
		{name: "clear with markdown ok", markdown: "# hi", clearAttachments: true, wantErr: ""},
		{name: "text and markdown conflict", text: "hi", markdown: "# hi", wantErr: "cannot be specified together"},
		{name: "nothing specified", wantErr: "specify --content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateEditContentFlags(tt.text, tt.markdown, tt.content, tt.attachments, tt.clearAttachments)
			if tt.wantErr == "" {
				if got != "" {
					t.Fatalf("validateEditContentFlags() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantErr) {
				t.Fatalf("validateEditContentFlags() = %q, want substring %q", got, tt.wantErr)
			}
		})
	}
}

// assertEditValidationError asserts the error is a typed validation error with
// the expected subtype and flag param (CodeRabbit: assert typed metadata, not
// just message text).
func assertEditValidationError(t *testing.T, err error, wantSub errs.Subtype, wantParam, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T, want *errs.ValidationError: %v", err, err)
	}
	if wantSub != "" && ve.Subtype != wantSub {
		t.Fatalf("subtype = %q, want %q", ve.Subtype, wantSub)
	}
	if wantParam != "" && ve.Param != wantParam {
		t.Fatalf("param = %q, want %q", ve.Param, wantParam)
	}
	if wantMsg != "" && !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("error = %v, want substring %q", err, wantMsg)
	}
}

// TestImMessagesEditValidate covers message-id, post-only attachment, clear/set mutual exclusion, and content-required validation.

func TestImMessagesEditValidate(t *testing.T) {
	t.Run("markdown with attachment", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id": "om_123",
			"markdown":   "# hi",
		}, map[string][]string{"set-attachments": {"file_1"}})
		if err := ImMessagesEdit.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesEdit.Validate() unexpected error = %v", err)
		}
	})

	t.Run("content post with attachment", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id": "om_123",
			"msg-type":   "post",
			"content":    `{"zh_cn":{"content":[[{"tag":"text","text":"hi"}]]}}`,
		}, map[string][]string{"set-attachments": {"file_1"}})
		if err := ImMessagesEdit.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesEdit.Validate() unexpected error = %v", err)
		}
	})

	t.Run("text with attachment rejected (post only)", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id": "om_123",
			"text":       "hi",
		}, map[string][]string{"set-attachments": {"file_1"}})
		err := ImMessagesEdit.Validate(context.Background(), rt)
		assertEditValidationError(t, err, errs.SubtypeInvalidArgument, "--set-attachments", "post message")
	})

	t.Run("missing message-id", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{"text": "hi"}, nil)
		err := ImMessagesEdit.Validate(context.Background(), rt)
		assertEditValidationError(t, err, errs.SubtypeInvalidArgument, "--message-id", "--message-id")
	})

	t.Run("missing content", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{"message-id": "om_123"}, nil)
		err := ImMessagesEdit.Validate(context.Background(), rt)
		assertEditValidationError(t, err, errs.SubtypeInvalidArgument, "", "specify --content")
	})

	t.Run("clear-attachments with markdown", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id":        "om_123",
			"markdown":          "# hi",
			"clear-attachments": "true",
		}, nil)
		if err := ImMessagesEdit.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesEdit.Validate() unexpected error = %v", err)
		}
	})

	t.Run("clear-attachments with attachment rejected", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id":        "om_123",
			"markdown":          "# hi",
			"clear-attachments": "true",
		}, map[string][]string{"set-attachments": {"file_1"}})
		err := ImMessagesEdit.Validate(context.Background(), rt)
		assertEditValidationError(t, err, errs.SubtypeInvalidArgument, "--clear-attachments", "cannot be used with --set-attachments")
	})

	t.Run("clear-attachments without post type rejected", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id":        "om_123",
			"text":              "hi",
			"clear-attachments": "true",
		}, nil)
		err := ImMessagesEdit.Validate(context.Background(), rt)
		assertEditValidationError(t, err, errs.SubtypeInvalidArgument, "--clear-attachments", "post message")
	})

	t.Run("clear-only with msg-type post ok", func(t *testing.T) {
		rt := newEditTestRuntimeContext(map[string]string{
			"message-id":        "om_123",
			"msg-type":          "post",
			"clear-attachments": "true",
		}, nil)
		if err := ImMessagesEdit.Validate(context.Background(), rt); err != nil {
			t.Fatalf("ImMessagesEdit.Validate() unexpected error = %v (clear-only edit must be allowed)", err)
		}
	})
}

// TestImMessagesEditDryRunPUTContract verifies the DryRun output for attachment
// set/clear flows: the HTTP method is PUT and the post body carries the files
// array exactly as Execute would send it. This is the regression guard that
// Validate's accepted inputs stay consistent with the actual request.
func TestImMessagesEditDryRunPUTContract(t *testing.T) {
	cases := []struct {
		name        string
		stringFlags map[string]string
		sliceFlags  map[string][]string
		wantMethod  string
		wantMsgType string
		wantFiles   string // substring expected in the content JSON
	}{
		{
			name:        "clear-only post edit sets files empty array",
			stringFlags: map[string]string{"message-id": "om_123", "msg-type": "post", "clear-attachments": "true"},
			wantMethod:  "PUT",
			wantMsgType: "post",
			wantFiles:   "\"files\":[]",
		},
		{
			name:        "set-attachments writes files array",
			stringFlags: map[string]string{"message-id": "om_123", "msg-type": "post"},
			sliceFlags:  map[string][]string{"set-attachments": {"file_1", "file_2"}},
			wantMethod:  "PUT",
			wantMsgType: "post",
			wantFiles:   "\"files\":[{\"key\":\"file_1\"},{\"key\":\"file_2\"}]",
		},
		{
			name:        "body-only edit has no files key",
			stringFlags: map[string]string{"message-id": "om_123", "markdown": "# hi"},
			wantMethod:  "PUT",
			wantMsgType: "post",
			wantFiles:   "", // absent: preserve semantics
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rt := newEditTestRuntimeContext(tt.stringFlags, tt.sliceFlags)
			raw := mustMarshalDryRun(t, ImMessagesEdit.DryRun(context.Background(), rt))
			var out struct {
				API []struct {
					Method string      `json:"method"`
					Body   interface{} `json:"body"`
				} `json:"api"`
			}
			if err := json.Unmarshal([]byte(raw), &out); err != nil {
				t.Fatalf("unmarshal dry-run JSON: %v\n%s", err, raw)
			}
			if len(out.API) != 1 {
				t.Fatalf("dry-run api calls = %d, want 1\n%s", len(out.API), raw)
			}
			if out.API[0].Method != tt.wantMethod {
				t.Fatalf("dry-run method = %q, want %q", out.API[0].Method, tt.wantMethod)
			}
			body, ok := out.API[0].Body.(map[string]interface{})
			if !ok {
				t.Fatalf("dry-run body = %#v, want object", out.API[0].Body)
			}
			if body["msg_type"] != tt.wantMsgType {
				t.Fatalf("dry-run msg_type = %v, want %q", body["msg_type"], tt.wantMsgType)
			}
			content, _ := body["content"].(string)
			if tt.wantFiles == "" {
				if strings.Contains(content, "\"files\"") {
					t.Fatalf("dry-run content unexpectedly has files key: %s", content)
				}
				return
			}
			if !strings.Contains(content, tt.wantFiles) {
				t.Fatalf("dry-run content = %s, want substring %q", content, tt.wantFiles)
			}
		})
	}
}
