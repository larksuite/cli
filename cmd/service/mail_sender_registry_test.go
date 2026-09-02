// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

const mailSenderRegistryFixture = `{
  "version": "1.0.0",
  "services": [{
    "name": "mail",
    "version": "v1",
    "servicePath": "/open-apis/mail/v1",
    "resources": {
      "user_mailbox.allow_senders": {"methods": {
        "list": {
          "id": "user_mailbox.allow_sender.list",
          "path": "user_mailboxes/{user_mailbox_id}/allow_senders",
          "httpMethod": "GET",
          "risk": "read",
          "scopes": ["mail:user_mailbox.message:readonly", "mail:user_mailbox.message:modify"],
          "accessTokens": ["tenant", "user"],
          "parameters": {
            "user_mailbox_id": {"type": "string", "location": "path", "required": true},
            "keyword": {"type": "string", "location": "query"},
            "page_size": {"type": "integer", "location": "query", "min": "1", "max": "100"},
            "page_token": {"type": "string", "location": "query"}
          }
        },
        "batch_create": {
          "id": "user_mailbox.allow_sender.batch_create",
          "path": "user_mailboxes/{user_mailbox_id}/allow_senders/batch_create",
          "httpMethod": "POST",
          "risk": "write",
          "scopes": ["mail:user_mailbox.message:modify"],
          "requiredScopes": ["mail:user_mailbox.message:modify"],
          "accessTokens": ["tenant", "user"],
          "parameters": {"user_mailbox_id": {"type": "string", "location": "path", "required": true}},
          "requestBody": {"items": {
            "type": "array",
            "required": true,
            "properties": {
              "sender": {"type": "string"},
              "sender_type": {"type": "integer", "min": "1", "max": "2"}
            }
          }}
        },
        "batch_remove": {
          "id": "user_mailbox.allow_sender.batch_remove",
          "path": "user_mailboxes/{user_mailbox_id}/allow_senders/batch_remove",
          "httpMethod": "POST",
          "risk": "write",
          "scopes": ["mail:user_mailbox.message:modify"],
          "requiredScopes": ["mail:user_mailbox.message:modify"],
          "accessTokens": ["tenant", "user"],
          "parameters": {"user_mailbox_id": {"type": "string", "location": "path", "required": true}},
          "requestBody": {"senders": {"type": "array", "required": true}}
        }
      }},
      "user_mailbox.blocked_senders": {"methods": {
        "list": {
          "id": "user_mailbox.blocked_sender.list",
          "path": "user_mailboxes/{user_mailbox_id}/blocked_senders",
          "httpMethod": "GET",
          "risk": "read",
          "scopes": ["mail:user_mailbox.message:readonly", "mail:user_mailbox.message:modify"],
          "accessTokens": ["tenant", "user"],
          "parameters": {
            "user_mailbox_id": {"type": "string", "location": "path", "required": true},
            "keyword": {"type": "string", "location": "query"},
            "page_size": {"type": "integer", "location": "query", "min": "1", "max": "100"},
            "page_token": {"type": "string", "location": "query"}
          }
        },
        "batch_create": {
          "id": "user_mailbox.blocked_sender.batch_create",
          "path": "user_mailboxes/{user_mailbox_id}/blocked_senders/batch_create",
          "httpMethod": "POST",
          "risk": "write",
          "scopes": ["mail:user_mailbox.message:modify"],
          "requiredScopes": ["mail:user_mailbox.message:modify"],
          "accessTokens": ["tenant", "user"],
          "parameters": {"user_mailbox_id": {"type": "string", "location": "path", "required": true}},
          "requestBody": {"items": {
            "type": "array",
            "required": true,
            "properties": {
              "sender": {"type": "string"},
              "sender_type": {"type": "integer", "min": "1", "max": "2"}
            }
          }}
        },
        "batch_remove": {
          "id": "user_mailbox.blocked_sender.batch_remove",
          "path": "user_mailboxes/{user_mailbox_id}/blocked_senders/batch_remove",
          "httpMethod": "POST",
          "risk": "write",
          "scopes": ["mail:user_mailbox.message:modify"],
          "requiredScopes": ["mail:user_mailbox.message:modify"],
          "accessTokens": ["tenant", "user"],
          "parameters": {"user_mailbox_id": {"type": "string", "location": "path", "required": true}},
          "requestBody": {"senders": {"type": "array", "required": true}}
        }
      }}
    }
  }]
}`

func TestMailSenderRegistrySnapshotLoadsAndDiscoversCommands(t *testing.T) {
	reg, err := meta.Parse([]byte(mailSenderRegistryFixture))
	if err != nil {
		t.Fatalf("parse registry fixture: %v", err)
	}
	if len(reg.Services) != 1 || reg.Services[0].Name != "mail" {
		t.Fatalf("services = %#v, want one mail service", reg.Services)
	}

	catalog := apicatalog.New(apicatalog.SourceEmbedded, reg.Services)
	root := &cobra.Command{Use: "lark-cli"}
	RegisterServiceCommandsFromCatalog(context.Background(), root, &cmdutil.Factory{}, catalog)

	resources := map[string]string{
		"user_mailbox.allow_senders":   "allow_senders",
		"user_mailbox.blocked_senders": "blocked_senders",
	}
	for resource, pathResource := range resources {
		resource, pathResource := resource, pathResource
		t.Run(resource, func(t *testing.T) {
			for _, methodName := range []string{"list", "batch_create", "batch_remove"} {
				cmd, _, err := root.Find([]string{"mail", resource, methodName})
				if err != nil || cmd.Name() != methodName {
					t.Fatalf("command %s.%s not discovered: cmd=%v err=%v", resource, methodName, cmd, err)
				}
				if got := cmd.Flags().Lookup("format").DefValue; got != "json" {
					t.Errorf("%s format default = %q, want json", methodName, got)
				}
				if cmd.Flags().Lookup("as") == nil || cmd.Flags().Lookup("user-mailbox-id") == nil {
					t.Errorf("%s missing --as or --user-mailbox-id", methodName)
				}
				if cmd.Flags().Lookup("params") == nil {
					t.Errorf("%s missing raw --params support", methodName)
				}
				if got := cmdutil.GetSupportedIdentities(cmd); !reflect.DeepEqual(got, []string{"bot", "user"}) {
					t.Errorf("%s identities = %v, want [bot user]", methodName, got)
				}

				target, err := catalog.Resolve([]string{"mail", resource, methodName})
				if err != nil || target.Method == nil {
					t.Fatalf("resolve %s.%s: target=%#v err=%v", resource, methodName, target, err)
				}
				method := target.Method.Method
				wantPath := "user_mailboxes/{user_mailbox_id}/" + pathResource
				if methodName != "list" {
					wantPath += "/" + methodName
				}
				if method.Path != wantPath {
					t.Errorf("%s path = %q, want %q", methodName, method.Path, wantPath)
				}

				switch methodName {
				case "list":
					if method.HTTPMethod != "GET" || cmd.Flags().Lookup("data") != nil {
						t.Errorf("list must be GET without --data")
					}
					for _, name := range []string{"keyword", "page-size", "page-token"} {
						if cmd.Flags().Lookup(name) == nil {
							t.Errorf("list missing --%s query flag", name)
						}
					}
					if !reflect.DeepEqual(method.Scopes, []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message:modify"}) {
						t.Errorf("list scopes = %v, want readonly OR modify", method.Scopes)
					}
					if len(method.RequiredScopes) != 0 {
						t.Errorf("list required scopes = %v, want OR semantics from scopes", method.RequiredScopes)
					}
					pageSize := method.Parameters["page_size"]
					if pageSize.Location != "query" || pageSize.Min != "1" || pageSize.Max != "100" {
						t.Errorf("page_size = %#v, want query range 1-100", pageSize)
					}
				case "batch_create":
					assertSenderWriteMethod(t, cmd, method, "items")
				case "batch_remove":
					assertSenderWriteMethod(t, cmd, method, "senders")
				}
			}
		})
	}
}

func assertSenderWriteMethod(t *testing.T, cmd *cobra.Command, method meta.Method, bodyField string) {
	t.Helper()
	if method.HTTPMethod != "POST" || cmd.Flags().Lookup("data") == nil {
		t.Errorf("%s must be POST with --data", method.Name)
	}
	data := method.Data()
	if len(data) != 1 || data[0].Name != bodyField || !data[0].Required {
		t.Errorf("%s body = %#v, want required %s", method.Name, data, bodyField)
	}
	if bodyField == "items" {
		children := data[0].Children()
		if len(children) != 2 || children[0].Name != "sender" || children[1].Name != "sender_type" {
			t.Errorf("%s items children = %#v, want sender and sender_type", method.Name, children)
		}
	}
	wantScope := []string{"mail:user_mailbox.message:modify"}
	if !reflect.DeepEqual(method.RequiredScopes, wantScope) {
		t.Errorf("%s required scopes = %v, want %v", method.Name, method.RequiredScopes, wantScope)
	}
	if risk, ok := cmdutil.GetRisk(cmd); !ok || risk != cmdutil.RiskWrite {
		t.Errorf("%s risk = %q, %v; want write", method.Name, risk, ok)
	}
}
