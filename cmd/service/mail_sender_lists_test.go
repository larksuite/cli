// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
	"github.com/spf13/cobra"
)

const (
	mailSenderReadScope  = "mail:user_mailbox.message:readonly"
	mailSenderWriteScope = "mail:user_mailbox.message:modify"
)

func mailSenderListService(t *testing.T) meta.Service {
	t.Helper()
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	svc, ok := snapshot.Catalog().Service("mail")
	if !ok {
		t.Fatal("mail service is missing from the embedded catalog")
	}
	return svc
}

func mailSenderListMethod(t *testing.T, svc meta.Service, resourceName, methodName string) meta.Method {
	t.Helper()
	resource, ok := svc.Resource(resourceName)
	if !ok {
		t.Fatalf("mail resource %q is missing", resourceName)
	}
	method, ok := resource.Method(methodName)
	if !ok {
		t.Fatalf("mail method %s.%s is missing", resourceName, methodName)
	}
	return method
}

func TestMailSenderListCommandsRegistered(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "lark-cli"}
	f := &cmdutil.Factory{APICatalog: snapshot.Catalog()}
	RegisterServiceCommandsForNames(context.Background(), root, f, f.APICatalog, []string{"mail"})

	for _, resource := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		for _, method := range []string{"batch_create", "list", "batch_remove"} {
			cmd, _, findErr := root.Find([]string{"mail", resource, method})
			if findErr != nil || cmd == root {
				t.Errorf("command mail %s %s is not registered: cmd=%v err=%v", resource, method, cmd, findErr)
				continue
			}
			for _, flag := range []string{"user-mailbox-id"} {
				if cmd.Flags().Lookup(flag) == nil {
					t.Errorf("command mail %s %s is missing --%s", resource, method, flag)
				}
			}
			if method == "list" {
				for _, flag := range []string{"keyword", "page-size", "page-token", "page-all"} {
					if cmd.Flags().Lookup(flag) == nil {
						t.Errorf("command mail %s list is missing --%s", resource, flag)
					}
				}
			} else if cmd.Flags().Lookup("data") == nil {
				t.Errorf("command mail %s %s is missing --data", resource, method)
			}
		}
	}
}

func TestMailSenderListRequestAndSuccessWorkflow(t *testing.T) {
	svc := mailSenderListService(t)
	for _, resourceName := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		resourceName := resourceName
		t.Run(resourceName, func(t *testing.T) {
			baseURL := "/open-apis/mail/v1/user_mailboxes/me/"
			segment := "allow_senders"
			if strings.Contains(resourceName, "blocked") {
				segment = "blocked_senders"
			}
			sender := "verify.example.com"

			create := mailSenderListMethod(t, svc, resourceName, "batch_create")
			createRequest, createOut := executeMailSenderListMethod(t, svc, resourceName, create,
				[]string{"--user-mailbox-id", "me", "--data", `{"items":[{"sender":"verify.example.com","sender_type":2}]}`},
				"POST", baseURL+segment+"/batch_create",
				map[string]interface{}{"failed_items": []interface{}{}})
			assertJSONBody(t, createRequest.stub.CapturedBody, map[string]interface{}{
				"items": []interface{}{map[string]interface{}{"sender": sender, "sender_type": float64(2)}},
			})
			assertSuccessData(t, createOut, "failed_items")

			list := mailSenderListMethod(t, svc, resourceName, "list")
			listRequest, listOut := executeMailSenderListMethod(t, svc, resourceName, list,
				[]string{"--user-mailbox-id", "me", "--keyword", sender, "--page-size", "20"},
				"GET", baseURL+segment,
				map[string]interface{}{
					"has_more": false,
					"items":    []interface{}{map[string]interface{}{"sender": sender}},
				})
			assertQuery(t, listRequest.query, url.Values{"keyword": {sender}, "page_size": {"20"}})
			assertOutputContains(t, listOut, sender)

			remove := mailSenderListMethod(t, svc, resourceName, "batch_remove")
			removeRequest, removeOut := executeMailSenderListMethod(t, svc, resourceName, remove,
				[]string{"--user-mailbox-id", "me", "--data", `{"senders":["verify.example.com"]}`},
				"POST", baseURL+segment+"/batch_remove",
				map[string]interface{}{"failed_items": []interface{}{}})
			assertJSONBody(t, removeRequest.stub.CapturedBody, map[string]interface{}{"senders": []interface{}{sender}})
			assertSuccessData(t, removeOut, "failed_items")

			_, finalOut := executeMailSenderListMethod(t, svc, resourceName, list,
				[]string{"--user-mailbox-id", "me", "--keyword", sender},
				"GET", baseURL+segment,
				map[string]interface{}{"has_more": false, "items": []interface{}{}})
			assertOutputOmitsSender(t, finalOut, sender)
		})
	}
}

func TestMailSenderListFailurePathsAndScopeBoundary(t *testing.T) {
	svc := mailSenderListService(t)
	for _, resourceName := range []string{"user_mailbox.allow_senders", "user_mailbox.blocked_senders"} {
		resourceName := resourceName
		t.Run(resourceName+" scopes", func(t *testing.T) {
			list := mailSenderListMethod(t, svc, resourceName, "list")
			if got := registry.DeclaredScopesForMethod(list, "user"); !reflect.DeepEqual(got, []string{mailSenderReadScope}) {
				t.Fatalf("list scopes = %v, want readonly only", got)
			}
			for _, methodName := range []string{"batch_create", "batch_remove"} {
				method := mailSenderListMethod(t, svc, resourceName, methodName)
				if got := registry.DeclaredScopesForMethod(method, "user"); !reflect.DeepEqual(got, []string{mailSenderWriteScope}) {
					t.Fatalf("%s scopes = %v, want modify only", methodName, got)
				}
			}
		})

		t.Run(resourceName+" missing mailbox", func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, testConfig)
			method := mailSenderListMethod(t, svc, resourceName, "list")
			cmd := NewCmdServiceMethod(f, svc, method, "list", resourceName, nil)
			cmd.SetArgs([]string{"--dry-run"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "missing required path parameter: user_mailbox_id") {
				t.Fatalf("missing mailbox error = %v", err)
			}
		})
	}

	t.Run("api error remains actionable", func(t *testing.T) {
		f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
		method := mailSenderListMethod(t, svc, "user_mailbox.allow_senders", "list")
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/mail/v1/user_mailboxes/me/allow_senders",
			Body: map[string]interface{}{
				"code": 230027,
				"msg":  "user not authorized",
			},
		})
		cmd := NewCmdServiceMethod(f, svc, method, "list", "user_mailbox.allow_senders", nil)
		cmd.SetArgs([]string{"--as", "bot", "--user-mailbox-id", "me"})
		err := cmd.Execute()
		var authorizationErr *errs.PermissionError
		if !errors.As(err, &authorizationErr) {
			t.Fatalf("error = %T %v, want *errs.PermissionError", err, err)
		}
		requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
		if strings.Contains(stdout.String(), `"ok": true`) || strings.Contains(stdout.String(), `"ok":true`) {
			t.Fatalf("API error was rendered as success: %s", stdout.String())
		}
	})
}

func executeMailSenderListMethod(
	t *testing.T,
	svc meta.Service,
	resourceName string,
	method meta.Method,
	args []string,
	httpMethod string,
	requestURL string,
	responseData map[string]interface{},
) (*mailRequestCapture, []byte) {
	t.Helper()
	f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
	capture := &mailRequestCapture{}
	stub := &httpmock.Stub{
		Method: httpMethod,
		URL:    requestURL,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": responseData,
		},
		OnMatch: func(req *http.Request) {
			capture.query = req.URL.Query()
		},
	}
	capture.stub = stub
	reg.Register(stub)
	cmd := NewCmdServiceMethod(f, svc, method, method.Name, resourceName, nil)
	cmd.SetArgs(append([]string{"--as", "bot"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s.%s: %v", resourceName, method.Name, err)
	}
	return capture, append([]byte(nil), stdout.Bytes()...)
}

type mailRequestCapture struct {
	stub  *httpmock.Stub
	query url.Values
}

func assertJSONBody(t *testing.T, body []byte, want map[string]interface{}) {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, body)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request body = %#v, want %#v", got, want)
	}
}

func assertQuery(t *testing.T, got, want url.Values) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query = %v, want %v", got, want)
	}
}

func assertSuccessData(t *testing.T, output []byte, key string) {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, output)
	}
	if envelope["ok"] != true {
		t.Fatalf("unexpected failure envelope: %#v", envelope)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %#v, want object", envelope["data"])
	}
	if _, ok := data[key]; !ok {
		t.Fatalf("data = %#v, want key %q", data, key)
	}
}

func assertOutputContains(t *testing.T, output []byte, sender string) {
	t.Helper()
	if !strings.Contains(string(output), sender) {
		t.Fatalf("success output does not contain sender %q: %s", sender, output)
	}
}

func assertOutputOmitsSender(t *testing.T, output []byte, sender string) {
	t.Helper()
	if strings.Contains(string(output), sender) {
		t.Fatalf("post-removal list still contains sender %q: %s", sender, output)
	}
}
