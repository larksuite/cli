// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestAllowBlockShortcutsAreRegistered(t *testing.T) {
	commands := map[string]bool{}
	for _, shortcut := range Shortcuts() {
		commands[shortcut.Command] = true
	}
	for _, want := range []string{"+allow-block-list", "+allow-block-search", "+allow-block-set", "+allow-block-delete"} {
		if !commands[want] {
			t.Fatalf("Shortcuts() missing %s", want)
		}
	}
}

func TestAllowBlockValidate(t *testing.T) {
	tests := []struct {
		name      string
		shortcut  common.Shortcut
		args      []string
		wantErr   string
		wantParam string
	}{
		{
			name:      "list rejects page size over cap",
			shortcut:  MailAllowBlockList,
			args:      []string{"+allow-block-list", "--page-size", "101"},
			wantErr:   "--page-size",
			wantParam: "--page-size",
		},
		{
			name:      "search requires query",
			shortcut:  MailAllowBlockSearch,
			args:      []string{"+allow-block-search", "--query", "   "},
			wantErr:   "--query",
			wantParam: "--query",
		},
		{
			name:      "search caps query length",
			shortcut:  MailAllowBlockSearch,
			args:      []string{"+allow-block-search", "--query", strings.Repeat("x", 256)},
			wantErr:   "255",
			wantParam: "--query",
		},
		{
			name:      "set forbids all type",
			shortcut:  MailAllowBlockSet,
			args:      []string{"+allow-block-set", "--type", "all", "--address", "a@example.com"},
			wantErr:   "--type",
			wantParam: "--type",
		},
		{
			name:      "delete requires address",
			shortcut:  MailAllowBlockDelete,
			args:      []string{"+allow-block-delete", "--type", "block"},
			wantErr:   "at least one sender",
			wantParam: "--address",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			assertValidationError(t, err, tc.wantErr)
			assertAllowBlockValidationProblem(t, err, tc.wantParam)
		})
	}
}

func assertAllowBlockValidationProblem(t *testing.T, err error, wantParam string) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed validation problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Fatalf("problem category = %q, want %q", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if got := ve.Param; got != wantParam {
		t.Fatalf("validation param = %q, want %q", got, wantParam)
	}
}

func TestAllowBlockDryRunUsesExistingAllowBlockedSenderAPIs(t *testing.T) {
	runtime := runtimeForAllowBlockDryRun(t, MailAllowBlockSearch, map[string]string{
		"mailbox":    "alice@example.com",
		"type":       "all",
		"query":      "vendor",
		"page-size":  "25",
		"page-token": "42",
	})

	dry := MailAllowBlockSearch.DryRun(context.Background(), runtime)
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry-run failed: %v", err)
	}
	s := string(raw)
	for _, want := range []string{
		`"method":"GET"`,
		`/user_mailboxes/alice@example.com/allow_senders`,
		`/user_mailboxes/alice@example.com/blocked_senders`,
		`"keyword":"vendor"`,
		`"page_size":25`,
		`"page_token":"42"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dry-run JSON missing %q; got:\n%s", want, s)
		}
	}
	if strings.Contains(s, "sender_allow_blocks") {
		t.Fatalf("dry-run must not invent sender_allow_blocks API; got:\n%s", s)
	}
}

func TestAllowBlockSetReadsAddressFileAndPostsItems(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("senders.txt", []byte("two@example.com\n\nOne@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, reg := mailShortcutTestFactory(t)
	var captured []string
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/allow_senders/batch_create",
		BodyFilter: func(body []byte) bool {
			var payload struct {
				Items []struct {
					Sender string `json:"sender"`
				} `json:"items"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				return false
			}
			for _, item := range payload.Items {
				captured = append(captured, item.Sender)
			}
			return true
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"failed_items": []interface{}{},
			},
		},
	})

	err := runMountedMailShortcut(t, MailAllowBlockSet, []string{
		"+allow-block-set", "--type", "allow", "--address", "One@example.com,two@example.com", "--address-file", "senders.txt",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	reg.Verify(t)
	want := []string{"One@example.com", "two@example.com"}
	if strings.Join(captured, ",") != strings.Join(want, ",") {
		t.Fatalf("captured senders = %v, want %v", captured, want)
	}
	out := decodeShortcutEnvelopeData(t, stdout)
	if got := int(out["requested"].(float64)); got != 2 {
		t.Fatalf("requested = %d, want 2; stdout=%s", got, stdout.String())
	}
}

func TestAllowBlockDeletePreservesAddressCase(t *testing.T) {
	tests := []struct {
		name             string
		body             interface{}
		rawBody          []byte
		wantDeletedCount int
	}{
		{
			name: "meta object response",
			body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"deleted_count": 1,
				},
			},
			wantDeletedCount: 1,
		},
		{
			name:             "boe non object success response",
			rawBody:          []byte("true"),
			wantDeletedCount: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, reg := mailShortcutTestFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "DELETE",
				URL:    "/user_mailboxes/me/blocked_senders/batch_delete",
				BodyFilter: func(body []byte) bool {
					var payload struct {
						Senders []string `json:"senders"`
					}
					if err := json.Unmarshal(body, &payload); err != nil {
						return false
					}
					return len(payload.Senders) == 1 && payload.Senders[0] == "MixedCase@Example.COM"
				},
				Body:    tc.body,
				RawBody: tc.rawBody,
			})

			err := runMountedMailShortcut(t, MailAllowBlockDelete, []string{
				"+allow-block-delete", "--type", "block", "--address", "MixedCase@Example.COM",
			}, f, stdout)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			reg.Verify(t)
			out := decodeShortcutEnvelopeData(t, stdout)
			if got := int(out["deleted_count"].(float64)); got != tc.wantDeletedCount {
				t.Fatalf("deleted_count = %d, want %d; stdout=%s", got, tc.wantDeletedCount, stdout.String())
			}
		})
	}
}

func TestAllowBlockListAggregatesAllowAndBlock(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/allow_senders",
		Body: allowBlockListResponse([]map[string]interface{}{
			{"sender": "safe@example.com", "create_time": "1"},
		}),
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/blocked_senders",
		Body: allowBlockListResponse([]map[string]interface{}{
			{"sender": "spam@example.com", "create_time": "2"},
		}),
	})

	err := runMountedMailShortcut(t, MailAllowBlockList, []string{"+allow-block-list", "--type", "all"}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	reg.Verify(t)
	out := decodeShortcutEnvelopeData(t, stdout)
	lists := out["lists"].(map[string]interface{})
	if _, ok := lists["allow"]; !ok {
		t.Fatalf("missing allow list in output: %s", stdout.String())
	}
	if _, ok := lists["block"]; !ok {
		t.Fatalf("missing block list in output: %s", stdout.String())
	}
}

func TestAllowBlockDecorateAPIErrorHints(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantHint  string
		retryable bool
	}{
		{
			name:      "cache empty",
			err:       errs.NewAPIError(errs.SubtypeUnknown, "ErrCacheEmpty").WithCode(456),
			wantHint:  "retry",
			retryable: true,
		},
		{
			name:     "self address",
			err:      errs.NewAPIError(errs.SubtypeUnknown, "MAIL_USER_ALLOW_BLOCK_SELF_ADDRESS"),
			wantHint: "own primary address",
		},
		{
			name:     "self domain",
			err:      errs.NewAPIError(errs.SubtypeUnknown, "MAIL_USER_ALLOW_BLOCK_SELF_DOMAIN"),
			wantHint: "internal tenant domains",
		},
		{
			name:     "data invalid",
			err:      errs.NewAPIError(errs.SubtypeUnknown, "MAIL_USER_ALLOW_BLOCK_DATA_INVALID"),
			wantHint: "100 senders",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := allowBlockDecorateAPIError(tc.err)
			if !errors.Is(got, tc.err) {
				t.Fatalf("decorated error should preserve original error")
			}
			p, ok := errs.ProblemOf(got)
			if !ok {
				t.Fatalf("expected typed problem, got %T", got)
			}
			if !strings.Contains(p.Hint, tc.wantHint) {
				t.Fatalf("hint = %q, want substring %q", p.Hint, tc.wantHint)
			}
			if p.Retryable != tc.retryable {
				t.Fatalf("retryable = %v, want %v", p.Retryable, tc.retryable)
			}
		})
	}
}

func runtimeForAllowBlockDryRun(t *testing.T, shortcut common.Shortcut, values map[string]string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, fl := range shortcut.Flags {
		switch fl.Type {
		case "int":
			cmd.Flags().Int(fl.Name, allowBlockDefaultPageSize, "")
		default:
			cmd.Flags().String(fl.Name, fl.Default, "")
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("parse flags failed: %v", err)
	}
	for k, v := range values {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag --%s failed: %v", k, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func allowBlockListResponse(items []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"items":      items,
			"has_more":   false,
			"page_token": "",
		},
	}
}
