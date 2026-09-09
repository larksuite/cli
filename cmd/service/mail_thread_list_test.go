// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
)

func mailThreadListSpec() (meta.Service, meta.Method) {
	service := meta.ServiceFromMap(map[string]interface{}{
		"name":        "mail",
		"servicePath": "/open-apis/mail/v1",
	})
	method := meta.FromMap(map[string]interface{}{
		"id":          mailThreadListMethodID,
		"path":        "user_mailboxes/{user_mailbox_id}/threads",
		"httpMethod":  "GET",
		"description": "List mail threads",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
			"page_size": map[string]interface{}{
				"type": "integer", "location": "query", "required": true,
			},
			"folder_id": map[string]interface{}{
				"type": "string", "location": "query", "required": false,
			},
			"label_id": map[string]interface{}{
				"type": "string", "location": "query", "required": false,
			},
		},
	})
	return service, method
}

func TestMailThreadListSelectorValidation(t *testing.T) {
	tests := []struct {
		name       string
		selectors  []string
		wantPhrase string
	}{
		{name: "neither", wantPhrase: "exactly one"},
		{name: "both", selectors: []string{"--folder-id", "INBOX", "--label-id", "FLAGGED"}, wantPhrase: "mutually exclusive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, testConfig)
			transportStub := &httpmock.Stub{
				Method:   "GET",
				URL:      "/open-apis/mail/v1/user_mailboxes/me/threads",
				Body:     map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
				Optional: true,
			}
			reg.Register(transportStub)
			configCalls := 0
			loadConfig := f.Config
			f.Config = func() (*core.CliConfig, error) {
				configCalls++
				return loadConfig()
			}
			service, method := mailThreadListSpec()
			cmd := NewCmdServiceMethod(f, service, method, "list", "user_mailbox.threads", nil)
			args := []string{"--user-mailbox-id", "me", "--page-size", "20"}
			cmd.SetArgs(append(args, tt.selectors...))

			err := cmd.Execute()
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
			}
			if validationErr.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tt.wantPhrase) {
				t.Fatalf("error = %q, want phrase %q", err, tt.wantPhrase)
			}
			if configCalls != 0 {
				t.Fatalf("config calls = %d, want 0 before validation", configCalls)
			}
			if got := len(transportStub.CapturedBodies); got != 0 {
				t.Fatalf("transport calls = %d, want 0", got)
			}
		})
	}
}

func TestMailThreadListSelectorValidRequests(t *testing.T) {
	tests := []struct {
		name      string
		selectors []string
		wantKey   string
		wantValue string
		absentKey string
	}{
		{name: "folder only", selectors: []string{"--folder-id", "INBOX"}, wantKey: "folder_id", wantValue: "INBOX", absentKey: "label_id"},
		{name: "label only via params", selectors: []string{"--params", `{"user_mailbox_id":"me","page_size":20,"label_id":"FLAGGED"}`}, wantKey: "label_id", wantValue: "FLAGGED", absentKey: "folder_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, testConfig)
			var calls int
			var query map[string]string
			stub := &httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/mail/v1/user_mailboxes/me/threads",
				Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
				OnMatch: func(req *http.Request) {
					calls++
					query = map[string]string{}
					for key, values := range req.URL.Query() {
						if len(values) > 0 {
							query[key] = values[0]
						}
					}
				},
			}
			reg.Register(stub)
			service, method := mailThreadListSpec()
			cmd := NewCmdServiceMethod(f, service, method, "list", "user_mailbox.threads", nil)
			args := []string{"--as", "bot"}
			if tt.name == "folder only" {
				args = append(args, "--user-mailbox-id", "me", "--page-size", "20")
			}
			cmd.SetArgs(append(args, tt.selectors...))

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if calls != 1 {
				t.Fatalf("transport calls = %d, want 1", calls)
			}
			if query[tt.wantKey] != tt.wantValue {
				t.Fatalf("query[%q] = %q, want %q", tt.wantKey, query[tt.wantKey], tt.wantValue)
			}
			if _, ok := query[tt.absentKey]; ok {
				t.Fatalf("query unexpectedly contains %q: %#v", tt.absentKey, query)
			}
			if query["page_size"] != "20" {
				t.Fatalf("page_size = %q, want 20", query["page_size"])
			}
		})
	}
}

func TestMailThreadListHelpDocumentsExactlyOneOptionalSelector(t *testing.T) {
	f := &cmdutil.Factory{}
	service, method := mailThreadListSpec()
	cmd := NewCmdServiceMethod(f, service, method, "list", "user_mailbox.threads", nil)
	PrepareMethodHelp(cmd, nil)
	var help bytes.Buffer
	cmd.SetOut(&help)
	cmd.SetErr(&help)
	if err := cmd.Help(); err != nil {
		t.Fatalf("Help() error: %v", err)
	}

	renderedHelp := strings.ToLower(help.String())
	if !strings.Contains(renderedHelp, "exactly one") || !strings.Contains(renderedHelp, "mutually exclusive") {
		t.Fatalf("rendered help does not describe the selector contract:\n%s", help.String())
	}
	for _, name := range []string{"folder-id", "label-id"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("missing --%s flag", name)
		}
		usage := strings.ToLower(flag.Usage)
		if !strings.Contains(usage, "exactly one") || !strings.Contains(usage, "mutually exclusive") {
			t.Fatalf("--%s help does not describe the selector contract: %q", name, flag.Usage)
		}
		if values := annotationOf(flag, flagSubAnnotation); len(values) != 1 || values[0] != subOptional {
			t.Fatalf("--%s subsection = %v, want Optional", name, values)
		}
	}
}
