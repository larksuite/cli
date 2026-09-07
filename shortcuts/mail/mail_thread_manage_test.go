// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func threadManageID(suffix string) string {
	return "thread_abcdefghijklmnop_" + suffix
}

func stubThreadManagePost(reg *httpmock.Registry, endpoint string, response map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/threads/" + endpoint,
		Body:   response,
	}
	reg.Register(stub)
	return stub
}

func decodeThreadManageBody(t *testing.T, stub *httpmock.Stub) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	return body
}

func TestThreadManage_MetadataAndRegistration(t *testing.T) {
	for _, shortcut := range []common.Shortcut{MailThreadModify, MailThreadTrash} {
		if strings.Join(shortcut.AuthTypes, ",") != "user,bot" {
			t.Errorf("%s AuthTypes = %v, want [user bot]", shortcut.Command, shortcut.AuthTypes)
		}
		if len(shortcut.Scopes) != 1 || shortcut.Scopes[0] != "mail:user_mailbox.message:modify" {
			t.Errorf("%s Scopes = %v", shortcut.Command, shortcut.Scopes)
		}
	}
	if MailThreadModify.Risk != "write" {
		t.Errorf("modify Risk = %q, want write", MailThreadModify.Risk)
	}
	if MailThreadTrash.Risk != "high-risk-write" {
		t.Errorf("trash Risk = %q, want high-risk-write", MailThreadTrash.Risk)
	}

	flags := map[string]common.Flag{}
	for _, flag := range MailThreadModify.Flags {
		flags[flag.Name] = flag
	}
	for _, name := range []string{"mailbox-id", "thread-id", "add-label-id", "remove-label-id", "folder-id"} {
		if _, ok := flags[name]; !ok {
			t.Fatalf("modify flags missing --%s", name)
		}
	}
	if flags["thread-id"].Required || flags["thread-id"].Type != "string_array" {
		t.Errorf("--thread-id = %#v, want validation-owned string_array", flags["thread-id"])
	}
	if got := strings.Join(flags["thread-id"].Aliases, ","); got != "thread-ids" {
		t.Errorf("--thread-id aliases = %q", got)
	}
	if got := strings.Join(flags["folder-id"].Aliases, ","); got != "add-folder" {
		t.Errorf("--folder-id aliases = %q", got)
	}

	commands := map[string]bool{}
	for _, shortcut := range Shortcuts() {
		commands[shortcut.Command] = true
	}
	for _, command := range []string{"+thread-modify", "+thread-trash"} {
		if !commands[command] {
			t.Fatalf("Shortcuts() missing %s", command)
		}
	}
}

func TestThreadManage_HelpShowsSingularFlags(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	parent := &cobra.Command{Use: "test"}
	MailThreadModify.Mount(parent, f)
	parent.SetOut(stdout)
	parent.SetErr(stdout)
	parent.SetArgs([]string{"+thread-modify", "--help"})
	err := parent.Execute()
	if err != nil {
		t.Fatalf("help error: %v", err)
	}
	for _, flag := range []string{"--thread-id", "--add-label-id", "--remove-label-id", "--folder-id", "--mailbox-id"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Errorf("help missing %s:\n%s", flag, stdout.String())
		}
	}
}

func TestThreadManage_NormalizeThreadIDs(t *testing.T) {
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	got, err := normalizeThreadManageIDs([]string{id1, id2, id1})
	if err != nil {
		t.Fatalf("normalize IDs: %v", err)
	}
	if strings.Join(got, ",") != id1+","+id2 {
		t.Fatalf("IDs = %v, want stable dedupe", got)
	}
	for _, raw := range [][]string{{}, {""}, {" "}, {id1 + ","}, {" " + id1}, {id1 + "\n" + id2}} {
		_, err := normalizeThreadManageIDs(raw)
		requireMessageManageValidationParam(t, err, "--thread-id")
	}
}

func TestThreadModify_OneRequestAndRawResponse(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"request_id": "req_1"},
	})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-id", id1,
		"--thread-id", id2,
		"--thread-id", id1,
		"--add-label-id", "unread",
		"--add-label-id", "customA",
		"--remove-label-id", "FLAGGED",
		"--folder-id", "  folderA  ",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute modify: %v", err)
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("POST count = %d, want 1", len(post.CapturedBodies))
	}
	body := decodeThreadManageBody(t, post)
	if got := body["thread_ids"].([]interface{}); len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("thread_ids = %#v, want stable dedupe", got)
	}
	if body["add_folder"] != "folderA" {
		t.Fatalf("add_folder = %#v, want trimmed folderA", body["add_folder"])
	}
	if got := body["add_label_ids"].([]interface{}); len(got) != 2 || got[0] != "UNREAD" || got[1] != "customA" {
		t.Fatalf("add_label_ids = %#v", got)
	}
	if got := body["remove_label_ids"].([]interface{}); len(got) != 1 || got[0] != "FLAGGED" {
		t.Fatalf("remove_label_ids = %#v", got)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["request_id"] != "req_1" {
		t.Fatalf("data = %#v, want raw downstream data", data)
	}
	for _, synthetic := range []string{"updated_count", "success_thread_ids", "failed_thread_ids"} {
		if _, ok := data[synthetic]; ok {
			t.Fatalf("synthetic field %q present in %#v", synthetic, data)
		}
	}
}

func TestThreadModify_OmitsOptionalFields(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--thread-id", threadManageID("1"), "--folder-id", "folderA",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute modify: %v", err)
	}
	body := decodeThreadManageBody(t, post)
	if body["add_folder"] != "folderA" {
		t.Fatalf("add_folder = %#v", body["add_folder"])
	}
	for _, omitted := range []string{"add_label_ids", "remove_label_ids"} {
		if _, ok := body[omitted]; ok {
			t.Fatalf("body must omit %s: %#v", omitted, body)
		}
	}
}

func TestThreadModify_OmitsFolderWhenNotProvided(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--thread-id", threadManageID("1"), "--add-label-id", "UNREAD",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute modify: %v", err)
	}
	body := decodeThreadManageBody(t, post)
	if _, ok := body["add_folder"]; ok {
		t.Fatalf("body must omit add_folder: %#v", body)
	}
}

func TestThreadModify_Validation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id := threadManageID("1")
	tests := []struct {
		name  string
		args  []string
		param string
	}{
		{name: "missing thread", args: []string{"+thread-modify", "--folder-id", "folderA"}, param: "thread-id"},
		{name: "missing action", args: []string{"+thread-modify", "--thread-id", id}, param: "thread-modify"},
		{name: "blank thread", args: []string{"+thread-modify", "--thread-id", " ", "--folder-id", "folderA"}, param: "thread-id"},
		{name: "blank add label", args: []string{"+thread-modify", "--thread-id", id, "--add-label-id", " "}, param: "add-label-id"},
		{name: "blank folder", args: []string{"+thread-modify", "--thread-id", id, "--folder-id", " "}, param: "folder-id"},
		{name: "label conflict", args: []string{"+thread-modify", "--thread-id", id, "--add-label-id", "unread", "--remove-label-id", "UNREAD"}, param: "add-label"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runMountedMailShortcut(t, MailThreadModify, test.args, f, stdout)
			var validation *errs.ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("error = %T %v, want ValidationError", err, err)
			}
			if !strings.Contains(validation.Param, test.param) {
				t.Fatalf("param = %q, want %q", validation.Param, test.param)
			}
		})
	}
}

func TestThreadModify_DryRunDoesNotCallAPI(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	post.Optional = true
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--thread-id", threadManageID("1"), "--add-label-id", "customA", "--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if len(post.CapturedBodies) != 0 {
		t.Fatalf("dry-run made %d API calls", len(post.CapturedBodies))
	}
	for _, want := range []string{"POST", "/user_mailboxes/me/threads/batch_modify", "thread_ids", "add_label_ids"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("dry-run missing %q: %s", want, stdout.String())
		}
	}
}

func TestThreadModify_APIFailurePassesThrough(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 1230001, "msg": "bad request"})
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--thread-id", threadManageID("1"), "--folder-id", "folderA",
	}, f, stdout)
	if err == nil || output.ExitCodeOf(err) != output.ExitAPI {
		t.Fatalf("error = %v, want API error", err)
	}
}

func TestThreadTrash_OneRequestAndRawResponse(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	post := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"job_id": "job_1"},
	})
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash", "--thread-id", id1, "--thread-id", id2, "--thread-id", id1, "--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute trash: %v", err)
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("POST count = %d, want 1", len(post.CapturedBodies))
	}
	body := decodeThreadManageBody(t, post)
	if len(body) != 1 {
		t.Fatalf("trash body includes modify-only fields: %#v", body)
	}
	if got := body["thread_ids"].([]interface{}); len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("thread_ids = %#v", got)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["job_id"] != "job_1" {
		t.Fatalf("data = %#v, want raw downstream data", data)
	}
}

func TestThreadTrash_ConfirmationDryRunAndEmptyResponse(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := threadManageID("1")
	post := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	post.Optional = true

	err := runMountedMailShortcut(t, MailThreadTrash, []string{"+thread-trash", "--thread-id", id}, f, stdout)
	if err == nil || output.ExitCodeOf(err) != output.ExitConfirmationRequired {
		t.Fatalf("error = %v, want confirmation required", err)
	}
	err = runMountedMailShortcut(t, MailThreadTrash, []string{"+thread-trash", "--thread-id", id, "--dry-run"}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if len(post.CapturedBodies) != 0 {
		t.Fatalf("dry-run made %d API calls", len(post.CapturedBodies))
	}
	post.Optional = false
	err = runMountedMailShortcut(t, MailThreadTrash, []string{"+thread-trash", "--thread-id", id, "--yes"}, f, stdout)
	if err != nil {
		t.Fatalf("execute trash: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if len(data) != 0 {
		t.Fatalf("empty downstream data gained synthetic fields: %#v", data)
	}
}

func TestThreadManage_LegacyAliasesRemainAccepted(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--mailbox", "me", "--thread-ids", threadManageID("1"),
		"--add-label-ids", "unread", "--remove-label-ids", "FLAGGED", "--add-folder", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("legacy aliases: %v", err)
	}
	body := decodeThreadManageBody(t, post)
	if body["add_folder"] != "ARCHIVED" {
		t.Fatalf("add_folder = %#v, want ARCHIVED", body["add_folder"])
	}
}
