// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/registry"
)

func TestMessageLinkQueryPreservesCardContent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := snapshot.Load("im")
	if err != nil {
		t.Fatal(err)
	}
	res, ok := svc.Resource("message_links")
	if !ok {
		t.Fatal("message_links missing from catalog")
	}
	method, ok := res.Method("query")
	if !ok {
		t.Fatal("query missing from catalog")
	}
	const link = "lark://applink.example.com/client/mail/forward/card?cardId=fixture&ownerId=fixture&threadId=a%2Fb%3D"
	const content = `{"title":"Shared mail","card_link":{"url":"` + link + `"},"elements":[[{"tag":"text","text":"Mail preview"}]]}`
	payload := map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{
			"message": map[string]interface{}{
				"message_id": "om_fixture", "msg_type": "interactive",
				"body": map[string]interface{}{"content": content},
			},
		}},
		"chat_info": map[string]interface{}{"chat_id": "oc_fixture", "is_authorized": false},
	}
	for _, format := range []string{"json", "jq"} {
		t.Run(format, func(t *testing.T) {
			f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
			reg.Register(&httpmock.Stub{
				URL:  "/open-apis/im/v1/message_links/query",
				Body: map[string]interface{}{"code": 0, "data": payload},
			})
			cmd := NewCmdServiceMethod(f, svc, method, "query", "message_links", nil)
			args := []string{"--as", "user", "--format", "json", "--data", `{"message_link_url":"https://applink.example.com/client/message/link/open?token=REPLACE_WITH_LINK_TOKEN"}`}
			if format == "jq" {
				args = append(args, "--jq", ".data.messages[].message.body.content | fromjson | .card_link.url")
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if format == "jq" {
				if stdout.String() != link+"\n" {
					t.Fatalf("card URL changed: %q, want %q", stdout.String(), link)
				}
				return
			}
			var got interface{}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("invalid output: %v: %s", err, stdout.String())
			}
			envelope, ok := got.(map[string]interface{})
			if !ok || envelope["ok"] != true || envelope["identity"] != "user" || !reflect.DeepEqual(envelope["data"], payload) {
				t.Fatalf("raw message content changed: %s", stdout.String())
			}
		})
	}
}
