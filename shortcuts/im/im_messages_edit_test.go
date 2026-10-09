// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
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

	// Identity lanes: the mounted shortcut accepts both --as user and --as bot
	// and plans the same PUT on the message edit endpoint, with no identity
	// error from the framework.
	for _, as := range []string{"user", "bot"} {
		t.Run("mounted dry-run as "+as, func(t *testing.T) {
			factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
				AppID: "test", AppSecret: "test", Brand: core.BrandFeishu, UserOpenId: "ou_self",
			})
			parent := &cobra.Command{Use: "im", SilenceErrors: true, SilenceUsage: true}
			ImMessagesEdit.Mount(parent, factory)
			parent.SetArgs([]string{"+messages-edit", "--as", as, "--message-id", "om_xxx", "--text", "hi", "--dry-run"})
			if err := parent.Execute(); err != nil {
				t.Fatalf("Execute(--as %s --dry-run) error = %v", as, err)
			}
			out := stdout.String()
			for _, want := range []string{`"PUT"`, "/open-apis/im/v1/messages/om_xxx"} {
				if !strings.Contains(out, want) {
					t.Fatalf("dry-run output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// TestImMessagesEditAuthTypesIncludeUser pins the identity declaration: the
// edit shortcut is available on both lanes, and the user lane requires the same
// scopes as +messages-send.
func TestImMessagesEditAuthTypesIncludeUser(t *testing.T) {
	for _, want := range []string{"bot", "user"} {
		if !slices.Contains(ImMessagesEdit.AuthTypes, want) {
			t.Fatalf("AuthTypes = %v, want it to include %q", ImMessagesEdit.AuthTypes, want)
		}
	}
	if !slices.Equal(ImMessagesEdit.UserScopes, ImMessagesSend.UserScopes) {
		t.Fatalf("UserScopes = %v, want %v (aligned with +messages-send)", ImMessagesEdit.UserScopes, ImMessagesSend.UserScopes)
	}
	if want := []string{"im:message.send_as_user", "im:message"}; !slices.Equal(ImMessagesEdit.UserScopes, want) {
		t.Fatalf("UserScopes = %v, want %v", ImMessagesEdit.UserScopes, want)
	}
	if want := []string{"im:message:send_as_bot"}; !slices.Equal(ImMessagesEdit.BotScopes, want) {
		t.Fatalf("BotScopes = %v, want %v", ImMessagesEdit.BotScopes, want)
	}
}

// identityTokenResolver returns a distinct token per token type so a test
// can tell from the Authorization header which identity a request used.
type identityTokenResolver struct{}

func (identityTokenResolver) ResolveToken(_ context.Context, spec credential.TokenSpec) (*credential.TokenResult, error) {
	if spec.Type == credential.TokenTypeUAT {
		return &credential.TokenResult{Token: "user-token"}, nil
	}
	return &credential.TokenResult{Token: "tenant-token"}, nil
}

// TestImMessagesEditExecuteUsesSelectedIdentity runs Execute end to end
// through the HTTP transport: the PUT on the edit endpoint must carry the
// token of the selected identity, so --as user really edits as the user.
func TestImMessagesEditExecuteUsesSelectedIdentity(t *testing.T) {
	for _, tc := range []struct {
		as        core.Identity
		wantToken string
	}{
		{core.AsUser, "Bearer user-token"},
		{core.AsBot, "Bearer tenant-token"},
	} {
		t.Run(string(tc.as), func(t *testing.T) {
			var gotMethod, gotPath, gotAuth string
			var gotBody map[string]interface{}
			runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotMethod, gotPath, gotAuth = req.Method, req.URL.Path, req.Header.Get("Authorization")
				if req.Body != nil {
					data, _ := io.ReadAll(req.Body)
					_ = json.Unmarshal(data, &gotBody)
				}
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"message_id": "om_xxx", "chat_id": "oc_xxx", "update_time": "1700000000000"},
				}), nil
			}))
			runtime.Factory.Credential = credential.NewCredentialProvider(nil, nil, identityTokenResolver{}, nil)
			setRuntimeField(t, runtime, "resolvedAs", tc.as)
			runtime.Cmd = newEditTestRuntimeContext(map[string]string{"message-id": "om_xxx", "text": "hi"}, nil).Cmd

			if err := ImMessagesEdit.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if gotMethod != http.MethodPut || gotPath != "/open-apis/im/v1/messages/om_xxx" {
				t.Fatalf("request = %s %s, want PUT /open-apis/im/v1/messages/om_xxx", gotMethod, gotPath)
			}
			if gotAuth != tc.wantToken {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tc.wantToken)
			}
			if gotBody["msg_type"] != "text" || gotBody["content"] != `{"text":"hi"}` {
				t.Fatalf("body = %v, want text content {\"text\":\"hi\"}", gotBody)
			}
		})
	}
}

// editScopedTokenResolver is identityTokenResolver with granted scopes attached,
// so the command entry's scope pre-check runs instead of being skipped.
type editScopedTokenResolver struct{ userScopes, botScopes string }

func (r editScopedTokenResolver) ResolveToken(_ context.Context, spec credential.TokenSpec) (*credential.TokenResult, error) {
	if spec.Type == credential.TokenTypeUAT {
		return &credential.TokenResult{Token: "user-token", Scopes: r.userScopes}, nil
	}
	return &credential.TokenResult{Token: "tenant-token", Scopes: r.botScopes}, nil
}

// TestImMessagesEditMountedExecuteAsUserAndBot drives the real command entry
// (identity resolution, scope pre-check and Execute) through the HTTP mock:
// --as user and --as bot each send one PUT on the edit endpoint, and the two
// requests carry the selected identity's token.
func TestImMessagesEditMountedExecuteAsUserAndBot(t *testing.T) {
	auth := map[string]string{}
	for _, as := range []string{"user", "bot"} {
		t.Run(as, func(t *testing.T) {
			factory, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
				AppID: "test", AppSecret: "test", Brand: core.BrandFeishu, UserOpenId: "ou_self",
			})
			factory.Credential = credential.NewCredentialProvider(nil, nil, editScopedTokenResolver{
				userScopes: "im:message.send_as_user im:message",
				botScopes:  "im:message:send_as_bot",
			}, nil)
			var gotBody map[string]interface{}
			reg.Register(&httpmock.Stub{
				Method: http.MethodPut,
				URL:    "/open-apis/im/v1/messages/om_xxx",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"message_id": "om_xxx", "chat_id": "oc_xxx", "update_time": "1700000000000"},
				},
				OnMatch: func(req *http.Request) {
					auth[as] = req.Header.Get("Authorization")
					if req.Body != nil {
						data, _ := io.ReadAll(req.Body)
						_ = json.Unmarshal(data, &gotBody)
					}
				},
			})
			parent := &cobra.Command{Use: "im", SilenceErrors: true, SilenceUsage: true}
			ImMessagesEdit.Mount(parent, factory)
			parent.SetArgs([]string{"+messages-edit", "--as", as, "--message-id", "om_xxx", "--text", "hi", "--format", "json"})
			if err := parent.Execute(); err != nil {
				t.Fatalf("Execute(--as %s) error = %v", as, err)
			}
			if auth[as] == "" {
				t.Fatalf("PUT /open-apis/im/v1/messages/om_xxx was not sent as %s", as)
			}
			if gotBody["msg_type"] != "text" || gotBody["content"] != `{"text":"hi"}` {
				t.Fatalf("body = %v, want text content {\"text\":\"hi\"}", gotBody)
			}
			if !strings.Contains(stdout.String(), "om_xxx") {
				t.Fatalf("output missing message_id:\n%s", stdout.String())
			}
		})
	}
	if auth["user"] != "Bearer user-token" || auth["bot"] != "Bearer tenant-token" {
		t.Fatalf("Authorization user=%q bot=%q, want user-token / tenant-token", auth["user"], auth["bot"])
	}
}

// TestImMessagesEditMountedUserMissingScopeSendsNothing checks that the
// command entry's scope pre-check guards the user lane: a user token without
// im:message.send_as_user fails before any request is sent.
func TestImMessagesEditMountedUserMissingScopeSendsNothing(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test", AppSecret: "test", Brand: core.BrandFeishu, UserOpenId: "ou_self",
	})
	factory.Credential = credential.NewCredentialProvider(nil, nil, editScopedTokenResolver{userScopes: "im:message"}, nil)
	sent := false
	reg.Register(&httpmock.Stub{
		Method:   http.MethodPut,
		URL:      "/open-apis/im/v1/messages/om_xxx",
		Body:     map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		Optional: true,
		OnMatch:  func(*http.Request) { sent = true },
	})
	parent := &cobra.Command{Use: "im", SilenceErrors: true, SilenceUsage: true}
	ImMessagesEdit.Mount(parent, factory)
	parent.SetArgs([]string{"+messages-edit", "--as", "user", "--message-id", "om_xxx", "--text", "hi"})
	err := parent.Execute()
	if err == nil || !strings.Contains(err.Error(), "im:message.send_as_user") {
		t.Fatalf("Execute() error = %v, want missing im:message.send_as_user", err)
	}
	if sent {
		t.Fatal("edit request was sent despite the missing scope")
	}
}
