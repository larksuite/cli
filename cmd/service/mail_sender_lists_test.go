// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/registry"
)

const (
	mailReadonlyScope = "mail:user_mailbox.message:readonly"
	mailModifyScope   = "mail:user_mailbox.message:modify"
)

func mailSenderMethod(t testing.TB, resource, method string) apicatalog.MethodRef {
	t.Helper()
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	target, err := snapshot.Catalog().Resolve([]string{"mail", resource, method})
	if err != nil {
		t.Fatalf("resolve mail.%s.%s: %v", resource, method, err)
	}
	if target.Kind != apicatalog.TargetMethod || target.Method == nil {
		t.Fatalf("mail.%s.%s resolved to %q, want method", resource, method, target.Kind)
	}
	return *target.Method
}

func executeMailSenderDryRun(t *testing.T, resource, method string, args ...string) map[string]any {
	t.Helper()
	ref := mailSenderMethod(t, resource, method)
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-mail-sender-lists", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	cmd := NewCmdServiceMethod(f, ref.Service, ref.Method, ref.MethodName(), ref.ResourceName(), nil)
	cmd.SetArgs(append(args, "--as", "user", "--dry-run"))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute mail.%s.%s dry-run: %v", resource, method, err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode dry-run output: %v\n%s", err, stdout.String())
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("dry-run data = %#v", envelope["data"])
	}
	calls, ok := data["api"].([]any)
	if !ok || len(calls) != 1 {
		t.Fatalf("dry-run calls = %#v, want one API call", data["api"])
	}
	call, ok := calls[0].(map[string]any)
	if !ok {
		t.Fatalf("dry-run call = %#v", calls[0])
	}
	return call
}

func TestMailSenderListsDryRun(t *testing.T) {
	for _, resource := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		t.Run(resource+"/list-all", func(t *testing.T) {
			call := executeMailSenderDryRun(t, resource, "list", "--user-mailbox-id", "me")
			if got, want := call["method"], "GET"; got != want {
				t.Fatalf("method = %v, want %s", got, want)
			}
			wantURL := "/open-apis/mail/v1/user_mailboxes/me/" + strings.TrimPrefix(resource, "user_mailbox.")
			if got := call["url"]; got != wantURL {
				t.Fatalf("url = %v, want %s", got, wantURL)
			}
			if params, ok := call["params"].(map[string]any); ok && len(params) != 0 {
				t.Fatalf("empty-query params = %#v, want omitted", params)
			}
		})

		t.Run(resource+"/search-page", func(t *testing.T) {
			call := executeMailSenderDryRun(t, resource, "list",
				"--user-mailbox-id", "me",
				"--keyword", "marketing",
				"--page-size", "50",
				"--page-token", "next-page",
			)
			want := map[string]any{"keyword": "marketing", "page_size": float64(50), "page_token": "next-page"}
			if got := call["params"]; !reflect.DeepEqual(got, want) {
				t.Fatalf("params = %#v, want %#v", got, want)
			}
		})

		t.Run(resource+"/batch-create", func(t *testing.T) {
			body := map[string]any{"items": []any{
				map[string]any{"sender": "trusted@example.com", "sender_type": float64(1)},
				map[string]any{"sender": "example.org", "sender_type": float64(2)},
			}}
			raw, _ := json.Marshal(body)
			call := executeMailSenderDryRun(t, resource, "batch_create",
				"--user-mailbox-id", "me", "--data", string(raw))
			if got, want := call["method"], "POST"; got != want {
				t.Fatalf("method = %v, want %s", got, want)
			}
			if got := call["body"]; !reflect.DeepEqual(got, body) {
				t.Fatalf("body = %#v, want %#v", got, body)
			}
		})

		t.Run(resource+"/batch-remove", func(t *testing.T) {
			body := map[string]any{"senders": []any{"trusted@example.com", "example.org"}}
			raw, _ := json.Marshal(body)
			call := executeMailSenderDryRun(t, resource, "batch_remove",
				"--user-mailbox-id", "me", "--data", string(raw))
			if got := call["body"]; !reflect.DeepEqual(got, body) {
				t.Fatalf("body = %#v, want %#v", got, body)
			}
		})
	}
}

type scopedTokenResolver struct{ scopes string }

func (r scopedTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-user-token", Scopes: r.scopes}, nil
}

func TestMailSenderListScopePreflight(t *testing.T) {
	config := &core.CliConfig{AppID: "test-mail-sender-scopes", Brand: core.BrandFeishu}
	for _, resource := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		for _, tc := range []struct {
			method string
			want   string
		}{
			{method: "list", want: mailReadonlyScope},
			{method: "batch_create", want: mailModifyScope},
			{method: "batch_remove", want: mailModifyScope},
		} {
			t.Run(resource+"/"+tc.method, func(t *testing.T) {
				method := mailSenderMethod(t, resource, tc.method).Method
				missing := credential.NewCredentialProvider(nil, nil, scopedTokenResolver{scopes: "mail:unrelated"}, nil)
				err := checkServiceScopes(context.Background(), missing, core.AsUser, config, method)
				var permissionErr *errs.PermissionError
				if !errors.As(err, &permissionErr) {
					t.Fatalf("missing scope error = %T %v, want PermissionError", err, err)
				}
				if !reflect.DeepEqual(permissionErr.MissingScopes, []string{tc.want}) {
					t.Fatalf("missing scopes = %v, want [%s]", permissionErr.MissingScopes, tc.want)
				}

				granted := credential.NewCredentialProvider(nil, nil, scopedTokenResolver{scopes: tc.want}, nil)
				if err := checkServiceScopes(context.Background(), granted, core.AsUser, config, method); err != nil {
					t.Fatalf("scope %s unexpectedly rejected: %v", tc.want, err)
				}
			})
		}
	}
}

func TestMailSenderServerErrorsRemainUnderstandable(t *testing.T) {
	messages := []string{
		"DATA_INVALID: invalid sender",
		"SELF_ADDRESS: the mailbox cannot list itself",
		"SELF_DOMAIN: the mailbox cannot list its own domain",
		"sender-list cache is not ready",
	}
	for _, message := range messages {
		t.Run(strings.Split(message, ":")[0], func(t *testing.T) {
			ref := mailSenderMethod(t, "user_mailbox.blocked_senders", "batch_create")
			f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
				AppID: "test-mail-sender-errors", AppSecret: "test-secret", Brand: core.BrandFeishu,
			})
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/mail/v1/user_mailboxes/me/blocked_senders/batch_create",
				Body:   map[string]any{"code": 190001, "msg": message},
			}
			reg.Register(stub)
			cmd := NewCmdServiceMethod(f, ref.Service, ref.Method, ref.MethodName(), ref.ResourceName(), nil)
			cmd.SetArgs([]string{
				"--as", "user",
				"--user-mailbox-id", "me",
				"--data", `{"items":[{"sender":"bad-format","sender_type":1}]}`,
			})
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected server error")
			}
			var apiErr *errs.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %T %v, want APIError", err, err)
			}
			if apiErr.Message != message || !strings.Contains(err.Error(), message) {
				t.Fatalf("server message = %q / %q, want %q", apiErr.Message, err.Error(), message)
			}
		})
	}
}

func TestMailSenderMethodsExposeUserIdentityAndBodyHelp(t *testing.T) {
	for _, resource := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		for _, methodName := range []string{"list", "batch_create", "batch_remove"} {
			ref := mailSenderMethod(t, resource, methodName)
			f := &cmdutil.Factory{}
			cmd := NewCmdServiceMethod(f, ref.Service, ref.Method, ref.MethodName(), ref.ResourceName(), nil)
			if got := cmd.Annotations["lark:supportedIdentities"]; got != "user" {
				t.Errorf("mail.%s.%s identities = %q, want user", resource, methodName, got)
			}
			if cmd.Flags().Lookup("user-mailbox-id") == nil {
				t.Errorf("mail.%s.%s missing --user-mailbox-id", resource, methodName)
			}
			if methodName == "list" {
				for _, flag := range []string{"keyword", "page-size", "page-token", "page-all"} {
					if cmd.Flags().Lookup(flag) == nil {
						t.Errorf("mail.%s.list missing --%s", resource, flag)
					}
				}
			} else if cmd.Flags().Lookup("data") == nil {
				t.Errorf("mail.%s.%s missing --data", resource, methodName)
			}
		}
	}
}
