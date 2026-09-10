// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func stubSenderPage(t *testing.T, reg *httpmock.Registry, typ, token, next string, more bool, senders ...string) {
	t.Helper()
	items := []map[string]string{}
	for _, sender := range senders {
		items = append(items, map[string]string{"sender": sender})
	}
	reg.Register(&httpmock.Stub{Method: "GET", URL: mailSenderPath(typ, ""), Body: map[string]any{"code": 0, "data": map[string]any{"items": items, "has_more": more, "page_token": next}}, OnMatch: func(req *http.Request) {
		if got := req.URL.Query().Get("page_token"); got != token {
			t.Errorf("page_token=%q want %q", got, token)
		}
		if req.URL.Query().Has("keyword") {
			t.Error("exact query must not use keyword search")
		}
		if req.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected Factory token: %q", req.Header.Get("Authorization"))
		}
	}})
}

func TestSenderGetCrossPageAndExactMatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, _, reg := mailShortcutTestFactory(t)
	stubSenderPage(t, reg, "allow", "", "next", true, "other-alice@example.com")
	stubSenderPage(t, reg, "allow", "next", "", false, " Alice@EXAMPLE.com ")
	stubSenderPage(t, reg, "block", "", "", false, "example.com")
	err := runMountedMailShortcut(t, MailSenderGet, []string{"+sender-get", "--sender", " alice@example.com "}, f, out)
	if err != nil {
		t.Fatal(err)
	}
	d := decodeShortcutEnvelopeData(t, out)
	if d["found"] != true || d["list_type"] != "allow" || d["sender"] != " Alice@EXAMPLE.com " {
		t.Fatalf("data=%#v", d)
	}
}

func TestSenderGetMissAndConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "miss", true: "conflict"}[conflict], func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _, reg := mailShortcutTestFactory(t)
			sender := "other@example.com"
			if conflict {
				sender = "alice@example.com"
			}
			stubSenderPage(t, reg, "allow", "", "", false, sender)
			stubSenderPage(t, reg, "block", "", "", false, sender)
			err := runMountedMailShortcut(t, MailSenderGet, []string{"+sender-get", "--sender", "alice@example.com"}, f, out)
			if conflict {
				var apiErr *errs.APIError
				if !errors.As(err, &apiErr) || apiErr.Subtype != errs.SubtypeConflict || out.Len() != 0 {
					t.Fatalf("error=%v output=%s", err, out)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			d := decodeShortcutEnvelopeData(t, out)
			if d["found"] != false || d["list_type"] != nil {
				t.Fatalf("data=%#v", d)
			}
		})
	}
}

func TestSenderListBothTypesAndEmpty(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "both", true: "empty"}[empty], func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _, reg := mailShortcutTestFactory(t)
			senders := []string{}
			if !empty {
				senders = []string{"alice@example.com"}
			}
			stubSenderPage(t, reg, "allow", "", "", false, senders...)
			stubSenderPage(t, reg, "block", "", "", false, senders...)
			err := runMountedMailShortcut(t, MailSenderList, []string{"+sender-list", "--page-size", "1"}, f, out)
			if err != nil {
				t.Fatal(err)
			}
			d := decodeShortcutEnvelopeData(t, out)
			items := d["items"].([]any)
			if empty {
				if len(items) != 0 {
					t.Fatal(items)
				}
				return
			}
			if len(items) != 2 || items[0].(map[string]any)["list_type"] != "allow" || items[1].(map[string]any)["list_type"] != "block" {
				t.Fatal(items)
			}
		})
	}
}

func TestSenderPaginationFailureNeverReportsMiss(t *testing.T) {
	for _, kind := range []string{"missing", "repeated", "malformed", "later_api_error"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _, reg := mailShortcutTestFactory(t)
			switch kind {
			case "missing":
				stubSenderPage(t, reg, "allow", "", "", true)
			case "repeated":
				stubSenderPage(t, reg, "allow", "", "loop", true)
				stubSenderPage(t, reg, "allow", "loop", "loop", true)
			case "malformed":
				reg.Register(&httpmock.Stub{Method: "GET", URL: mailSenderPath("allow", ""), Body: map[string]any{"code": 0, "data": map[string]any{"items": []any{}}}})
			case "later_api_error":
				stubSenderPage(t, reg, "allow", "", "next", true, "alice@example.com")
				reg.Register(&httpmock.Stub{Method: "GET", URL: mailSenderPath("allow", ""), Body: map[string]any{"code": 12345, "msg": "dependency failed"}})
			}
			err := runMountedMailShortcut(t, MailSenderGet, []string{"+sender-get", "--sender", "alice@example.com"}, f, out)
			if err == nil || output.ExitCodeOf(err) == 0 || out.Len() != 0 {
				t.Fatalf("error=%v output=%s", err, out)
			}
			if _, ok := errs.ProblemOf(err); !ok {
				t.Fatalf("untyped error: %v", err)
			}
		})
	}
}

func TestSenderSetAndDeleteRequestsAndBusinessFailures(t *testing.T) {
	for _, sc := range []common.Shortcut{MailSenderSet, MailSenderDelete} {
		for _, typ := range []string{"allow", "block"} {
			for _, failed := range []bool{false, true} {
				t.Run(sc.Command+"/"+typ+"/"+map[bool]string{false: "success", true: "failed_items"}[failed], func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, out, _, reg := mailShortcutTestFactory(t)
					action, status := "batch_create", "configured"
					var want any = map[string]any{"items": []any{map[string]any{"sender": "Alice@example.com"}}}
					if sc.Command == "+sender-delete" {
						action, status = "batch_remove", "absent"
						want = map[string]any{"senders": []any{"Alice@example.com"}}
					}
					failures := []map[string]any{}
					if failed {
						failures = append(failures, map[string]any{"sender": "Alice@example.com", "reason_code": 123})
					}
					stub := &httpmock.Stub{Method: "POST", URL: mailSenderPath(typ, action), Body: map[string]any{"code": 0, "data": map[string]any{"failed_items": failures}}}
					reg.Register(stub)
					args := []string{sc.Command, "--sender", " Alice@example.com ", "--type", typ}
					if sc.Command == "+sender-delete" {
						args = append(args, "--yes")
					}
					err := runMountedMailShortcut(t, sc, args, f, out)
					var body any
					if e := json.Unmarshal(stub.CapturedBody, &body); e != nil {
						t.Fatal(e)
					}
					if !reflect.DeepEqual(body, want) {
						t.Fatalf("body=%#v want=%#v", body, want)
					}
					if stub.CapturedHeaders.Get("Authorization") != "Bearer test-token" {
						t.Fatal("write must use Factory credentials")
					}
					if failed {
						var ae *errs.APIError
						if !errors.As(err, &ae) || ae.Code != 123 || ae.Hint == "" || out.Len() != 0 {
							t.Fatalf("error=%v output=%s", err, out)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					d := decodeShortcutEnvelopeData(t, out)
					if d["status"] != status || d["list_type"] != typ {
						t.Fatal(d)
					}
				})
			}
		}
	}
}

func TestSenderMetadataAndDryRun(t *testing.T) {
	registered := map[string]bool{}
	for _, s := range Shortcuts() {
		registered[s.Command] = true
	}
	for _, sc := range []common.Shortcut{MailSenderList, MailSenderGet, MailSenderSet, MailSenderDelete} {
		t.Run(sc.Command, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _, _ := mailShortcutTestFactory(t)
			if !registered[sc.Command] || !reflect.DeepEqual(sc.AuthTypes, []string{"user"}) {
				t.Fatal("registration/auth", sc)
			}
			scope := "mail:user_mailbox.message:readonly"
			_ = scope
			args := []string{sc.Command, "--dry-run"}
			if sc.Command != "+sender-list" {
				args = append(args, "--sender", "alice@example.com")
			}
			if sc.Command == "+sender-set" || sc.Command == "+sender-delete" {
				args = append(args, "--type", "block")
				scope = "mail:user_mailbox.message:modify"
			}
			if !reflect.DeepEqual(sc.Scopes, []string{scope}) {
				t.Fatal(sc.Scopes)
			}
			if err := runMountedMailShortcut(t, sc, args, f, out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "/user_mailboxes/me/") {
				t.Fatal(out.String())
			}
		})
	}
}

func TestSenderValidationAndBotRejection(t *testing.T) {
	cases := []struct {
		sc    common.Shortcut
		args  []string
		param string
	}{
		{MailSenderGet, []string{"--sender", " "}, "--sender"},
		{MailSenderSet, []string{"--sender", "alice@example.com", "--type", "all"}, "--type"},
		{MailSenderList, []string{"--page-size", "0"}, "--page-size"},
		{MailSenderList, []string{"--page-size", "101"}, "--page-size"},
		{MailSenderList, []string{"--type", "unknown"}, "--type"},
	}
	for _, tc := range cases {
		t.Run(tc.param+strings.Join(tc.args, ""), func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.sc, append([]string{tc.sc.Command}, tc.args...), f, out)
			var ve *errs.ValidationError
			if !errors.As(err, &ve) || ve.Param != tc.param {
				t.Fatalf("error=%#v", err)
			}
		})
	}
	t.Run("bot", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
		f, out, _, _ := mailShortcutTestFactory(t)
		err := runMountedMailShortcut(t, MailSenderList, []string{"+sender-list", "--as", "bot"}, f, out)
		if err == nil || out.Len() != 0 {
			t.Fatalf("error=%v out=%s", err, out)
		}
	})
}

func TestSenderPermissionErrorPreserved(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: mailSenderPath("allow", ""), Status: 403, Body: map[string]any{"code": 99991679, "msg": "Access denied. Required scope: mail:user_mailbox.message:readonly", "log_id": "sender-permission-log"}})
	err := runMountedMailShortcut(t, MailSenderList, []string{"+sender-list", "--type", "allow"}, f, out)
	var pe *errs.PermissionError
	if !errors.As(err, &pe) || pe.Code != 99991679 || pe.LogID != "sender-permission-log" || out.Len() != 0 {
		t.Fatalf("error=%#v out=%s", err, out)
	}
}

func TestSenderListSingleTypeAndPageSize(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: mailSenderPath("block", ""), Body: map[string]any{"code": 0, "data": map[string]any{"items": []map[string]string{{"sender": "example.com"}}, "has_more": false}}, OnMatch: func(req *http.Request) {
		if req.URL.Query().Get("page_size") != "17" {
			t.Fatal(req.URL)
		}
	}})
	if err := runMountedMailShortcut(t, MailSenderList, []string{"+sender-list", "--type", "block", "--page-size", "17"}, f, out); err != nil {
		t.Fatal(err)
	}
	data := decodeShortcutEnvelopeData(t, out)
	if len(data["items"].([]any)) != 1 {
		t.Fatal(data)
	}
}

func TestSenderDeleteRequiresConfirmation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSenderDelete, []string{"+sender-delete", "--sender", "alice@example.com", "--type", "block"}, f, out)
	var ce *errs.ConfirmationRequiredError
	if !errors.As(err, &ce) || out.Len() != 0 {
		t.Fatalf("error=%#v output=%s", err, out)
	}
}
