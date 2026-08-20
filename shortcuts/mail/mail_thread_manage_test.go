// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func threadManageID(suffix string) string {
	return "thread_abcdefghijklmnop_" + suffix
}

func threadManageIDs(count int) []string {
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		ids = append(ids, threadManageID(fmt.Sprintf("%02d", i+1)))
	}
	return ids
}

func stubThreadManagePost(reg *httpmock.Registry, endpoint string, body map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/threads/" + endpoint,
		Body:   body,
	}
	reg.Register(stub)
	return stub
}

func decodeThreadManageSummary(t *testing.T, data map[string]interface{}) ([]interface{}, []interface{}) {
	t.Helper()
	success, ok := data["success_thread_ids"].([]interface{})
	if !ok {
		t.Fatalf("success_thread_ids = %#v, want array", data["success_thread_ids"])
	}
	failed, ok := data["failed_thread_ids"].([]interface{})
	if !ok {
		t.Fatalf("failed_thread_ids = %#v, want array", data["failed_thread_ids"])
	}
	return success, failed
}

func TestThreadModify_Metadata(t *testing.T) {
	if MailThreadModify.Command != "+thread-modify" {
		t.Fatalf("Command = %q", MailThreadModify.Command)
	}
	if MailThreadModify.Risk != "write" {
		t.Errorf("Risk = %q, want write", MailThreadModify.Risk)
	}
	if len(MailThreadModify.AuthTypes) != 1 || MailThreadModify.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailThreadModify.AuthTypes)
	}
	if len(MailThreadModify.Scopes) != 1 || MailThreadModify.Scopes[0] != "mail:user_mailbox.message:modify" {
		t.Errorf("Scopes = %v, want [mail:user_mailbox.message:modify]", MailThreadModify.Scopes)
	}
	if len(MailThreadModify.ConditionalScopes) != 0 {
		t.Errorf("ConditionalScopes = %v, want none", MailThreadModify.ConditionalScopes)
	}
	flags := map[string]common.Flag{}
	for _, fl := range MailThreadModify.Flags {
		flags[fl.Name] = fl
	}
	for _, name := range []string{"mailbox", "thread-ids", "add-label-ids", "remove-label-ids", "add-folder"} {
		if _, ok := flags[name]; !ok {
			t.Fatalf("missing --%s flag", name)
		}
	}
	if flags["thread-ids"].Type != "string_array" || !flags["thread-ids"].Required {
		t.Errorf("--thread-ids = %#v, want required string_array", flags["thread-ids"])
	}
	if got := strings.Join(flags["add-folder"].Aliases, ","); got != "folder-id" {
		t.Errorf("--add-folder aliases = %q, want folder-id", got)
	}
}

func TestThreadTrash_Metadata(t *testing.T) {
	if MailThreadTrash.Command != "+thread-trash" {
		t.Fatalf("Command = %q", MailThreadTrash.Command)
	}
	if MailThreadTrash.Risk != "high-risk-write" {
		t.Errorf("Risk = %q, want high-risk-write", MailThreadTrash.Risk)
	}
	if len(MailThreadTrash.AuthTypes) != 1 || MailThreadTrash.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", MailThreadTrash.AuthTypes)
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

func TestThreadManage_NormalizeThreadIDs(t *testing.T) {
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	got, err := normalizeThreadManageIDs([]string{id1 + "," + id2, id1})
	if err != nil {
		t.Fatalf("normalizeThreadManageIDs returned error: %v", err)
	}
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("ids = %v, want [%s %s]", got, id1, id2)
	}
	for _, tc := range [][]string{
		{},
		{""},
		{" "},
		{id1 + ","},
		{" " + id1},
		{id1 + "\n" + id2},
		{"1234567890123456"},
		{"thread_abcdefghijklmnop!"},
	} {
		_, err := normalizeThreadManageIDs(tc)
		requireMessageManageValidationParam(t, err, "--thread-ids")
	}
	withDuplicates := append(threadManageIDs(mailThreadManageBatchSize), threadManageID("01"))
	got, err = normalizeThreadManageIDs(withDuplicates)
	if err != nil {
		t.Fatalf("normalizeThreadManageIDs with duplicate returned error: %v", err)
	}
	if len(got) != mailThreadManageBatchSize {
		t.Fatalf("ids len = %d, want %d after dedupe", len(got), mailThreadManageBatchSize)
	}
}

func TestThreadModify_LabelFolderBodyAndOutputContract(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id1 + "," + id2 + "," + id1,
		"--add-label-ids", "unread,customA",
		"--remove-label-ids", "FLAGGED",
		"--add-folder", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	threadIDs := body["thread_ids"].([]interface{})
	if len(threadIDs) != 2 || threadIDs[0] != id1 || threadIDs[1] != id2 {
		t.Fatalf("thread_ids = %#v, want deduped [%s %s]", threadIDs, id1, id2)
	}
	if got := body["add_folder"]; got != "ARCHIVED" {
		t.Fatalf("add_folder = %v, want ARCHIVED", got)
	}
	addLabels := body["add_label_ids"].([]interface{})
	if addLabels[0] != "UNREAD" || addLabels[1] != "customA" {
		t.Fatalf("add_label_ids = %#v, want [UNREAD customA]", addLabels)
	}
	removeLabels := body["remove_label_ids"].([]interface{})
	if len(removeLabels) != 1 || removeLabels[0] != "FLAGGED" {
		t.Fatalf("remove_label_ids = %#v, want [FLAGGED]", removeLabels)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	success, failed := decodeThreadManageSummary(t, data)
	if len(success) != 2 || success[0] != id1 || success[1] != id2 || len(failed) != 0 {
		t.Fatalf("summary success=%v failed=%v, want [%s %s]/[]", success, failed, id1, id2)
	}
	if _, ok := data["updated_count"]; ok {
		t.Fatalf("updated_count must not be present: %#v", data)
	}
	if _, ok := data["failed_ids"]; ok {
		t.Fatalf("failed_ids must not be present: %#v", data)
	}
	if _, ok := data["submitted_thread_ids"]; ok {
		t.Fatalf("submitted_thread_ids must not be present: %#v", data)
	}
	if _, ok := data["submitted_count"]; ok {
		t.Fatalf("submitted_count must not be present: %#v", data)
	}
}

func TestThreadModify_FolderIDAliasMapsToAddFolder(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := threadManageID("alias")
	post := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--folder-id", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if got := body["add_folder"]; got != "ARCHIVED" {
		t.Fatalf("add_folder = %v, want ARCHIVED", got)
	}
}

func TestThreadModify_Validation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id := threadManageID("1")
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--thread-modify")
	if !strings.Contains(err.Error(), "provide at least one") {
		t.Fatalf("error = %v, want missing action validation", err)
	}

	err = runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--add-label-ids", "unread",
		"--remove-label-ids", "UNREAD",
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--add-label-ids")

	err = runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--folder-id", "trash",
	}, f, stdout)
	requireMessageManageValidationParam(t, err, "--folder-id")
	if !strings.Contains(err.Error(), "use +thread-trash") {
		t.Fatalf("error = %v, want +thread-trash hint", err)
	}
}

func TestThreadModify_BatchesAndAggregatesPartialFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	ids := threadManageIDs(41)
	first := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	second := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 1230001, "msg": "bad request"})
	third := stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", strings.Join(ids, ","),
		"--add-folder", "archive",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for idx, stub := range []*httpmock.Stub{first, second, third} {
		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("batch %d body unmarshal: %v", idx+1, err)
		}
		threadIDs := body["thread_ids"].([]interface{})
		want := []int{20, 20, 1}[idx]
		if len(threadIDs) != want {
			t.Fatalf("batch %d size = %d, want %d", idx+1, len(threadIDs), want)
		}
		if body["add_folder"] != "ARCHIVED" {
			t.Fatalf("batch %d add_folder = %v, want ARCHIVED", idx+1, body["add_folder"])
		}
	}
	success, failed := decodeThreadManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 21 || len(failed) != 20 {
		t.Fatalf("success=%d failed=%d, want 21/20", len(success), len(failed))
	}
}

func TestThreadModify_AllBatchesFailReturnsError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := threadManageID("1")
	stubThreadManagePost(reg, "batch_modify", map[string]interface{}{"code": 1230001, "msg": "bad request"})

	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id,
		"--add-folder", "archive",
	}, f, stdout)
	requireMessageManageFailedPrecondition(t, err)
}

func TestThreadModify_DryRunShowsPostURLAndBody(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	err := runMountedMailShortcut(t, MailThreadModify, []string{
		"+thread-modify",
		"--thread-ids", id1 + "," + id2,
		"--add-label-ids", "customA",
		"--add-folder", "folderA",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/threads/batch_modify`,
		`thread_ids`,
		`add_label_ids`,
		`add_folder`,
		`batch_size`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
	if strings.Contains(out, `validation_api_plan`) {
		t.Fatalf("dry-run output must not include validation_api_plan; got %s", out)
	}
}

func TestThreadTrash_RequiresYesAndBatches(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id1 := threadManageID("1")
	id2 := threadManageID("2")
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id1 + "," + id2,
	}, f, stdout)
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if code := output.ExitCodeOf(err); code != output.ExitConfirmationRequired {
		t.Fatalf("exit code = %d, want %d", code, output.ExitConfirmationRequired)
	}

	post := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	err = runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id1 + "," + id2,
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err with --yes: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(post.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if got := len(body["thread_ids"].([]interface{})); got != 2 {
		t.Fatalf("thread_ids len = %d, want 2", got)
	}
	success, failed := decodeThreadManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 2 || len(failed) != 0 {
		t.Fatalf("summary success=%v failed=%v", success, failed)
	}
}

func TestThreadTrash_AllBatchesFailReturnsError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	id := threadManageID("1")
	stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 1230001, "msg": "bad request"})

	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id,
		"--yes",
	}, f, stdout)
	requireMessageManageFailedPrecondition(t, err)
}

func TestThreadTrash_BatchesAndAggregatesPartialFailure(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	ids := threadManageIDs(41)
	first := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
	second := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 1230001, "msg": "bad request"})
	third := stubThreadManagePost(reg, "batch_trash", map[string]interface{}{"code": 0, "data": map[string]interface{}{}})

	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", strings.Join(ids, ","),
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for idx, stub := range []*httpmock.Stub{first, second, third} {
		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("batch %d body unmarshal: %v", idx+1, err)
		}
		threadIDs := body["thread_ids"].([]interface{})
		want := []int{20, 20, 1}[idx]
		if len(threadIDs) != want {
			t.Fatalf("batch %d size = %d, want %d", idx+1, len(threadIDs), want)
		}
	}
	success, failed := decodeThreadManageSummary(t, decodeShortcutEnvelopeData(t, stdout))
	if len(success) != 21 || len(failed) != 20 {
		t.Fatalf("success=%d failed=%d, want 21/20", len(success), len(failed))
	}
}

func TestThreadTrash_DryRunShowsPostURLAndBody(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	id := threadManageID("1")
	err := runMountedMailShortcut(t, MailThreadTrash, []string{
		"+thread-trash",
		"--thread-ids", id,
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`/user_mailboxes/me/threads/batch_trash`,
		`thread_ids`,
		`batch_size`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
}
