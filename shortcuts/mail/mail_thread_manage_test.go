// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func stubThreadManagePost(reg *httpmock.Registry, endpoint string, body map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/threads/" + endpoint,
		Body:   body,
	}
	reg.Register(stub)
	return stub
}

func TestThreadManage_MetadataMatchesRegistry(t *testing.T) {
	for _, shortcut := range []common.Shortcut{MailThreadModify, MailThreadTrash} {
		if strings.Join(shortcut.AuthTypes, ",") != "user,bot" {
			t.Errorf("%s AuthTypes = %v, want [user bot]", shortcut.Command, shortcut.AuthTypes)
		}
		if len(shortcut.Scopes) != 1 || shortcut.Scopes[0] != "mail:user_mailbox.message:modify" {
			t.Errorf("%s Scopes = %v", shortcut.Command, shortcut.Scopes)
		}
	}
	if MailThreadModify.Risk != "write" || MailThreadTrash.Risk != "high-risk-write" {
		t.Fatalf("risks = %q/%q", MailThreadModify.Risk, MailThreadTrash.Risk)
	}

	flags := map[string]common.Flag{}
	for _, flag := range MailThreadModify.Flags {
		flags[flag.Name] = flag
	}
	for _, name := range []string{"mailbox", "thread-id", "add-label-id", "remove-label-id", "folder-id"} {
		if _, ok := flags[name]; !ok {
			t.Fatalf("missing --%s", name)
		}
	}
	if flags["thread-id"].Type != "string_slice" || !flags["thread-id"].Required {
		t.Fatalf("--thread-id = %#v, want required string_slice", flags["thread-id"])
	}
	for _, forbidden := range []string{"thread-ids", "add-label-ids", "remove-label-ids", "add-folder", "data"} {
		if _, ok := flags[forbidden]; ok {
			t.Fatalf("unexpected compatibility bypass --%s", forbidden)
		}
	}
	trashFlags := map[string]common.Flag{}
	for _, flag := range MailThreadTrash.Flags {
		trashFlags[flag.Name] = flag
	}
	if len(trashFlags) != 2 || trashFlags["thread-id"].Type != "string_slice" || !trashFlags["thread-id"].Required {
		t.Fatalf("trash flags = %#v, want mailbox plus required string_slice thread-id", trashFlags)
	}
}

func TestThreadManage_ShortcutsRegistration(t *testing.T) {
	commands := map[string]bool{}
	for _, shortcut := range Shortcuts() {
		commands[shortcut.Command] = true
	}
	for _, want := range []string{"+thread-modify", "+thread-trash"} {
		if !commands[want] {
			t.Fatalf("Shortcuts() missing %s", want)
		}
	}
}

func TestNormalizeThreadIDsTrimsDeduplicatesAndRejectsEmpty(t *testing.T) {
	got, err := normalizeThreadIDs([]string{" first ,second", "first", " third "}, "--thread-id", true)
	if err != nil {
		t.Fatalf("normalizeThreadIDs: %v", err)
	}
	want := []string{"first", "second", "third"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for _, raw := range [][]string{nil, {""}, {"first,"}, {" , "}} {
		_, err := normalizeThreadIDs(raw, "--thread-id", true)
		requireMessageManageValidationParam(t, err, "--thread-id")
	}
	if got, err := normalizeThreadIDs(nil, "--add-label-id", false); err != nil || got != nil {
		t.Fatalf("optional empty ids = %v, %v", got, err)
	}
}

func TestBuildThreadModifyRequestUsesExplicitWhitelist(t *testing.T) {
	req, err := buildThreadModifyRequest("shared@example.com", threadModifyInput{
		ThreadIDs:        []string{" thread-a,thread-b ", "thread-a"},
		AddLabelIDs:      []string{" label-a ", "label-a"},
		RemoveLabelIDs:   []string{" label-b "},
		FolderID:         " folder-x ",
		FolderIDProvided: true,
	})
	if err != nil {
		t.Fatalf("buildThreadModifyRequest: %v", err)
	}
	if req.Method != "POST" || req.Path != "/open-apis/mail/v1/user_mailboxes/shared@example.com/threads/batch_modify" {
		t.Fatalf("method/path = %s %s", req.Method, req.Path)
	}
	if len(req.Body) != 4 {
		t.Fatalf("body keys = %#v, want exactly four allowlisted keys", req.Body)
	}
	if got := req.Body["add_folder"]; got != "folder-x" {
		t.Fatalf("add_folder = %#v", got)
	}
	if got := strings.Join(req.Body["thread_ids"].([]string), ","); got != "thread-a,thread-b" {
		t.Fatalf("thread_ids = %q", got)
	}

	omitted, err := buildThreadModifyRequest("me", threadModifyInput{
		ThreadIDs:   []string{"thread-a"},
		AddLabelIDs: []string{"label-a"},
	})
	if err != nil {
		t.Fatalf("build omitted request: %v", err)
	}
	if len(omitted.Body) != 2 {
		t.Fatalf("omitted body = %#v", omitted.Body)
	}
	if _, ok := omitted.Body["remove_label_ids"]; ok {
		t.Fatal("empty remove_label_ids must be omitted")
	}
	if _, ok := omitted.Body["add_folder"]; ok {
		t.Fatal("empty add_folder must be omitted")
	}
}

func TestBuildThreadModifyRequestValidation(t *testing.T) {
	base := threadModifyInput{ThreadIDs: []string{"thread-a"}}
	_, err := buildThreadModifyRequest("me", base)
	requireMessageManageValidationParam(t, err, "--thread-modify")

	conflict := base
	conflict.AddLabelIDs = []string{" label-a "}
	conflict.RemoveLabelIDs = []string{"label-a"}
	_, err = buildThreadModifyRequest("me", conflict)
	requireMessageManageValidationParam(t, err, "--add-label-id")

	emptyFolder := base
	emptyFolder.FolderIDProvided = true
	emptyFolder.FolderID = "  "
	_, err = buildThreadModifyRequest("me", emptyFolder)
	requireMessageManageValidationParam(t, err, "--folder-id")
}

func TestThreadManageRejectsBlankMailboxBeforeRequest(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "modify",
			shortcut: MailThreadModify,
			args:     []string{"+thread-modify", "--mailbox", "   ", "--thread-id", "thread-a", "--add-label-id", "FLAGGED"},
		},
		{
			name:     "trash",
			shortcut: MailThreadTrash,
			args:     []string{"+thread-trash", "--mailbox", "   ", "--thread-id", "thread-a", "--yes"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			requireMessageManageValidationParam(t, err, "--mailbox")
		})
	}
}

func TestThreadManageMailboxNormalizationPreservesDefault(t *testing.T) {
	modify, err := buildThreadModifyRequest(" me ", threadModifyInput{
		ThreadIDs:   []string{"thread-a"},
		AddLabelIDs: []string{"FLAGGED"},
	})
	if err != nil {
		t.Fatalf("build modify request: %v", err)
	}
	if modify.Path != "/open-apis/mail/v1/user_mailboxes/me/threads/batch_modify" {
		t.Fatalf("modify path = %q", modify.Path)
	}

	trash, err := buildThreadTrashRequest(" me ", []string{"thread-a"})
	if err != nil {
		t.Fatalf("build trash request: %v", err)
	}
	if trash.Path != "/open-apis/mail/v1/user_mailboxes/me/threads/batch_trash" {
		t.Fatalf("trash path = %q", trash.Path)
	}
}

func TestThreadModifyExecuteCallsOnceAndPassesThroughData(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"operation_id": "op-server", "accepted": true},
	})
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-id", " thread-a , thread-b ",
		"--thread-id", "thread-a",
		"--add-label-id", " label-a ",
		"--remove-label-id", "label-b",
		"--folder-id", " folder-x ",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("requests = %d, want 1", len(post.CapturedBodies))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if len(body) != 4 || body["add_folder"] != "folder-x" {
		t.Fatalf("request body = %#v", body)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["operation_id"] != "op-server" || data["accepted"] != true {
		t.Fatalf("data = %#v", data)
	}
	for _, forbidden := range []string{"updated_count", "success_thread_ids", "failed_thread_ids", "submitted_count"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("synthetic response field %q in %#v", forbidden, data)
		}
	}
}

func TestThreadTrashExecuteCallsOnceWithOnlyThreadIDs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	post := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{"status": "accepted"},
	})
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash", "--thread-id", " thread-a,thread-b ", "--thread-id", "thread-a", "--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(post.CapturedBodies) != 1 {
		t.Fatalf("requests = %d, want 1", len(post.CapturedBodies))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("trash body = %#v, want only thread_ids", body)
	}
	if got := strings.Join(interfaceSliceStrings(body["thread_ids"]), ","); got != "thread-a,thread-b" {
		t.Fatalf("thread_ids = %q", got)
	}
	if data := decodeShortcutEnvelopeData(t, stdout); data["status"] != "accepted" {
		t.Fatalf("data = %#v", data)
	}
}

func TestThreadManageFieldProjectsSuccessEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		endpoint string
		args     []string
	}{
		{
			name:     "modify",
			shortcut: MailThreadModify,
			endpoint: "batch_modify",
			args:     []string{"+thread-modify", "--thread-id", "thread-a", "--add-label-id", "FLAGGED", "--format", "json", "--field", "ok"},
		},
		{
			name:     "trash",
			shortcut: MailThreadTrash,
			endpoint: "batch_trash",
			args:     []string{"+thread-trash", "--thread-id", "thread-a", "--yes", "--format", "json", "--field", "ok"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			post := stubThreadManagePost(reg, tc.endpoint, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"status": "accepted"},
			})

			if err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(post.CapturedBodies) != 1 {
				t.Fatalf("requests = %d, want 1", len(post.CapturedBodies))
			}
			if got := strings.TrimSpace(stdout.String()); got != "true" {
				t.Fatalf("--field ok output = %q, want true", got)
			}
		})
	}
}

func TestThreadManageDryRunUsesSameRequestShape(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--thread-id", " thread-a,thread-b ", "--add-label-id", "label-a", "--folder-id", " folder-x ", "--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"/user_mailboxes/me/threads/batch_modify", "thread_ids", "add_label_ids", "add_folder", "folder-x"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run missing %q: %s", want, out)
		}
	}
	for _, forbidden := range []string{"batch_size", "batches", "success_thread_ids"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("dry-run contains synthetic %q: %s", forbidden, out)
		}
	}
}

func TestThreadTrashDryRunContainsOnlyOneAllowlistedRequest(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash", "--thread-id", " thread-a,thread-b ", "--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"/user_mailboxes/me/threads/batch_trash", "thread_ids", "thread-a", "thread-b"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run missing %q: %s", want, out)
		}
	}
	for _, forbidden := range []string{"batch_size", "batches", "add_label_ids", "add_folder"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("dry-run contains forbidden %q: %s", forbidden, out)
		}
	}
}

func TestThreadModifyNetworkFailureIsNotRetried(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method:   "POST",
		URL:      "/user_mailboxes/me/threads/batch_modify",
		Error:    errors.New("request timed out"),
		Reusable: true,
	}
	reg.Register(stub)
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify", "--thread-id", "thread-a", "--add-label-id", "label-a",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected transport error")
	}
	if len(stub.CapturedBodies) != 1 {
		t.Fatalf("calls = %d, want 1", len(stub.CapturedBodies))
	}
}

func TestThreadManageRejectsBypassFlags(t *testing.T) {
	for _, flag := range []string{"--data", "--add-folder", "--thread-ids", "--add-label-ids", "--remove-label-ids"} {
		f, stdout, _, _ := mailShortcutTestFactory(t)
		err := runMountedMailShortcut(t, MailThreadModify, []string{
			"+thread-modify", "--thread-id", "thread-a", "--folder-id", "folder-x", flag, "{}",
		}, f, stdout)
		if err == nil || !strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("%s error = %v, want unknown flag", flag, err)
		}
	}
}

func TestThreadTrashRequiresConfirmation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailThreadTrash, []string{"+thread-trash", "--thread-id", "thread-a"}, f, stdout)
	if output.ExitCodeOf(err) != output.ExitConfirmationRequired {
		t.Fatalf("error = %v, code = %d", err, output.ExitCodeOf(err))
	}
}

func interfaceSliceStrings(value interface{}) []string {
	items, _ := value.([]interface{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.(string))
	}
	return out
}
