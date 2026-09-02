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
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
	"github.com/spf13/cobra"
)

const (
	mailMessageReadonly = "mail:user_mailbox.message:readonly"
	mailMessageModify   = "mail:user_mailbox.message:modify"
)

type mailSenderMethodContract struct {
	resource       string
	method         string
	httpMethod     string
	path           string
	bodyField      string
	responseField  string
	requiredScopes []string
}

var mailSenderMethodContracts = []mailSenderMethodContract{
	{
		resource:       "user_mailbox.allow_senders",
		method:         "list",
		httpMethod:     "GET",
		path:           "user_mailboxes/{user_mailbox_id}/allow_senders",
		responseField:  "items",
		requiredScopes: []string{mailMessageReadonly, mailMessageModify},
	},
	{
		resource:       "user_mailbox.allow_senders",
		method:         "batch_create",
		httpMethod:     "POST",
		path:           "user_mailboxes/{user_mailbox_id}/allow_senders/batch_create",
		bodyField:      "items",
		responseField:  "failed_items",
		requiredScopes: []string{mailMessageModify},
	},
	{
		resource:       "user_mailbox.allow_senders",
		method:         "batch_remove",
		httpMethod:     "POST",
		path:           "user_mailboxes/{user_mailbox_id}/allow_senders/batch_remove",
		bodyField:      "senders",
		responseField:  "deleted_count",
		requiredScopes: []string{mailMessageModify},
	},
	{
		resource:       "user_mailbox.blocked_senders",
		method:         "list",
		httpMethod:     "GET",
		path:           "user_mailboxes/{user_mailbox_id}/blocked_senders",
		responseField:  "items",
		requiredScopes: []string{mailMessageReadonly, mailMessageModify},
	},
	{
		resource:       "user_mailbox.blocked_senders",
		method:         "batch_create",
		httpMethod:     "POST",
		path:           "user_mailboxes/{user_mailbox_id}/blocked_senders/batch_create",
		bodyField:      "items",
		responseField:  "failed_items",
		requiredScopes: []string{mailMessageModify},
	},
	{
		resource:       "user_mailbox.blocked_senders",
		method:         "batch_remove",
		httpMethod:     "POST",
		path:           "user_mailboxes/{user_mailbox_id}/blocked_senders/batch_remove",
		bodyField:      "senders",
		responseField:  "deleted_count",
		requiredScopes: []string{mailMessageModify},
	},
}

func generatedMailService(t *testing.T) meta.Service {
	t.Helper()
	for _, service := range registry.EmbeddedServicesTyped() {
		if service.Name == "mail" {
			return service
		}
	}
	t.Fatal("generated registry does not contain the mail service; run make fetch_meta")
	return meta.Service{}
}

func generatedMailSenderMethod(t *testing.T, service meta.Service, resourceName, methodName string) meta.Method {
	t.Helper()
	resource, ok := service.Resource(resourceName)
	if !ok {
		t.Fatalf("generated mail registry does not contain resource %q", resourceName)
	}
	method, ok := resource.Method(methodName)
	if !ok {
		t.Fatalf("generated mail registry does not contain method %s.%s", resourceName, methodName)
	}
	return method
}

func TestGeneratedMailUserSenderMetadataContract(t *testing.T) {
	service := generatedMailService(t)
	for _, contract := range mailSenderMethodContracts {
		contract := contract
		t.Run(contract.resource+"/"+contract.method, func(t *testing.T) {
			method := generatedMailSenderMethod(t, service, contract.resource, contract.method)
			if method.HTTPMethod != contract.httpMethod || method.Path != contract.path {
				t.Fatalf("transport = %s %s, want %s %s", method.HTTPMethod, method.Path, contract.httpMethod, contract.path)
			}
			if len(method.AccessTokens) != 2 {
				t.Fatalf("access tokens = %v, want tenant and user", method.AccessTokens)
			}
			if got := []string{string(method.AccessTokens[0]), string(method.AccessTokens[1])}; !reflect.DeepEqual(got, []string{"tenant", "user"}) {
				t.Fatalf("access tokens = %v, want tenant and user", got)
			}
			if !reflect.DeepEqual(method.RequiredScopes, contract.requiredScopes) {
				t.Fatalf("required scopes = %v, want %v", method.RequiredScopes, contract.requiredScopes)
			}

			mailbox, ok := method.Parameters["user_mailbox_id"]
			if !ok || mailbox.Location != "path" || !mailbox.Required {
				t.Fatalf("user_mailbox_id = %+v, want required path parameter", mailbox)
			}
			if contract.method == "list" {
				for _, name := range []string{"keyword", "page_size", "page_token"} {
					if field, ok := method.Parameters[name]; !ok || field.Location != "query" || field.Required {
						t.Errorf("%s = %+v, want optional query parameter", name, field)
					}
				}
			} else {
				body, ok := method.RequestBody[contract.bodyField]
				if !ok || !body.Required {
					t.Fatalf("request body field %q = %+v, want required", contract.bodyField, body)
				}
			}
			if _, ok := method.ResponseBody[contract.responseField]; !ok {
				t.Fatalf("response is missing %q; write responses must preserve the published result shape", contract.responseField)
			}
		})
	}
}

func TestGeneratedMailUserSenderCommandsHelpAndRequiredMailbox(t *testing.T) {
	service := generatedMailService(t)
	root := &cobra.Command{Use: "lark-cli"}
	registerService(root, service, &cmdutil.Factory{})

	for _, contract := range mailSenderMethodContracts {
		contract := contract
		t.Run(contract.resource+"/"+contract.method, func(t *testing.T) {
			cmd, _, err := root.Find([]string{"mail", contract.resource, contract.method})
			if err != nil || cmd.Name() != contract.method {
				t.Fatalf("generated command mail %s %s not found: %v", contract.resource, contract.method, err)
			}
			if cmd.Flags().Lookup("user-mailbox-id") == nil {
				t.Fatal("generated command is missing --user-mailbox-id")
			}
			if contract.method == "list" {
				for _, name := range []string{"keyword", "page-size", "page-token", "dry-run"} {
					if cmd.Flags().Lookup(name) == nil {
						t.Errorf("generated list command is missing --%s", name)
					}
				}
			} else {
				for _, name := range []string{"data", "dry-run", "yes"} {
					if cmd.Flags().Lookup(name) == nil {
						t.Errorf("generated write command is missing --%s", name)
					}
				}
			}

			f, _, _, _ := cmdutil.TestFactory(t, testConfig)
			standalone := NewCmdServiceMethod(f, service,
				generatedMailSenderMethod(t, service, contract.resource, contract.method),
				contract.method, contract.resource, nil)
			standalone.SilenceErrors = true
			standalone.SilenceUsage = true
			standalone.SetArgs([]string{"--as", "bot", "--dry-run"})
			err = standalone.Execute()
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("missing mailbox error = %T %v, want *errs.ValidationError", err, err)
			}
			if validationErr.Param != "user_mailbox_id" ||
				!strings.Contains(validationErr.Hint, "--user-mailbox-id") ||
				!strings.Contains(validationErr.Hint, "lark-cli schema mail."+contract.resource+"."+contract.method) {
				t.Fatalf("missing mailbox error is not actionable: %+v", validationErr)
			}
		})
	}
}

func TestGeneratedMailUserSenderDryRunRequests(t *testing.T) {
	service := generatedMailService(t)
	for _, contract := range mailSenderMethodContracts {
		contract := contract
		t.Run(contract.resource+"/"+contract.method, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
			cmd := NewCmdServiceMethod(f, service,
				generatedMailSenderMethod(t, service, contract.resource, contract.method),
				contract.method, contract.resource, nil)
			args := []string{"--as", "bot", "--user-mailbox-id", "me", "--dry-run"}
			switch contract.method {
			case "list":
				args = append(args, "--keyword", "example", "--page-size", "20")
			case "batch_create":
				args = append(args, "--data", `{"items":[{"sender":"spam.example","sender_type":2}]}`)
			case "batch_remove":
				args = append(args, "--data", `{"senders":["spam.example"]}`)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("dry-run failed: %v", err)
			}

			var envelope struct {
				OK     bool `json:"ok"`
				DryRun bool `json:"dry_run"`
				Data   struct {
					API []struct {
						Method string                 `json:"method"`
						URL    string                 `json:"url"`
						Params map[string]interface{} `json:"params"`
						Body   map[string]interface{} `json:"body"`
					} `json:"api"`
				} `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("dry-run output is not JSON: %v\n%s", err, stdout.String())
			}
			if !envelope.OK || !envelope.DryRun || len(envelope.Data.API) != 1 {
				t.Fatalf("unexpected dry-run envelope: %s", stdout.String())
			}
			call := envelope.Data.API[0]
			wantURL := "/open-apis/mail/v1/" + strings.Replace(contract.path, "{user_mailbox_id}", "me", 1)
			if call.Method != contract.httpMethod || call.URL != wantURL {
				t.Fatalf("dry-run transport = %s %s, want %s %s", call.Method, call.URL, contract.httpMethod, wantURL)
			}
			if contract.method == "list" {
				if call.Params["keyword"] != "example" || call.Params["page_size"] != float64(20) {
					t.Fatalf("list params = %#v, want keyword and page_size", call.Params)
				}
			} else if _, ok := call.Body[contract.bodyField]; !ok {
				t.Fatalf("write body = %#v, want %q", call.Body, contract.bodyField)
			}
		})
	}
}

type mailSenderScopeTokenResolver struct {
	scopes string
}

func (r *mailSenderScopeTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "user-access-token", Scopes: r.scopes}, nil
}

func TestGeneratedMailUserSenderPreflightScopes(t *testing.T) {
	cfg := &core.CliConfig{
		AppID:      "mail-sender-scope-test",
		AppSecret:  "secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_mail_sender_scope_test",
	}
	resolver := &mailSenderScopeTokenResolver{}
	provider := credential.NewCredentialProvider(nil, nil, resolver, nil)

	service := generatedMailService(t)
	list := generatedMailSenderMethod(t, service, "user_mailbox.allow_senders", "list")
	resolver.scopes = mailMessageModify
	err := checkServiceScopes(context.Background(), provider, core.AsUser, cfg, list)
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("list preflight error = %T %v, want *errs.PermissionError", err, err)
	}
	if !reflect.DeepEqual(permissionErr.MissingScopes, []string{mailMessageReadonly}) {
		t.Fatalf("list missing scopes = %v, want readonly", permissionErr.MissingScopes)
	}

	resolver.scopes = mailMessageReadonly + " " + mailMessageModify
	if err := checkServiceScopes(context.Background(), provider, core.AsUser, cfg, list); err != nil {
		t.Fatalf("list preflight rejected both required scopes: %v", err)
	}
	write := generatedMailSenderMethod(t, service, "user_mailbox.blocked_senders", "batch_create")
	resolver.scopes = mailMessageModify
	if err := checkServiceScopes(context.Background(), provider, core.AsUser, cfg, write); err != nil {
		t.Fatalf("write preflight rejected modify scope: %v", err)
	}
}

func TestGeneratedMailUserSenderResourceNamesStayDotted(t *testing.T) {
	service := generatedMailService(t)
	root := &cobra.Command{Use: "lark-cli"}
	registerService(root, service, &cmdutil.Factory{})
	mail, _, err := root.Find([]string{"mail"})
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		found := false
		for _, child := range mail.Commands() {
			if child.Name() == resource {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mail help is missing generated resource %q", resource)
		}
	}
}
