// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func mailRuleReorderTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())

	cfg := mailTestConfig()
	token := &auth.StoredUAToken{
		UserOpenId:       cfg.UserOpenId,
		AppId:            cfg.AppID,
		AccessToken:      "test-user-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        time.Now().Add(1 * time.Hour).UnixMilli(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
		Scope:            "mail:user_mailbox.rule:read mail:user_mailbox.rule:write",
		GrantedAt:        time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	t.Cleanup(func() {
		_ = auth.RemoveStoredToken(cfg.AppID, cfg.UserOpenId)
	})

	return cmdutil.TestFactory(t, cfg)
}

func TestRuleReorderExecuteCompletesPartialOrder(t *testing.T) {
	f, stdout, _, reg := mailRuleReorderTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body:   ruleListResponse("r1", "r2", "r3", "r4"),
	})
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/user_mailboxes/me/rules/reorder",
		BodyFilter: requestRuleIDsEqual([]string{"r3", "r1", "r2", "r4"}),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder", "--rule-id", "r3", "--rule-id", "r1",
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	assertStringSliceField(t, data, "requested_rule_ids", []string{"r3", "r1"})
	assertStringSliceField(t, data, "appended_rule_ids", []string{"r2", "r4"})
	assertStringSliceField(t, data, "final_rule_ids", []string{"r3", "r1", "r2", "r4"})
	if got := int(data["total"].(float64)); got != 4 {
		t.Fatalf("total = %d, want 4", got)
	}
	if got := data["mailbox"]; got != "me" {
		t.Fatalf("mailbox = %v, want me", got)
	}
}

func TestRuleReorderExecuteKeepsCompleteInput(t *testing.T) {
	f, stdout, _, reg := mailRuleReorderTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body:   ruleListResponse("r1", "r2", "r3"),
	})
	reg.Register(&httpmock.Stub{
		Method:     "POST",
		URL:        "/user_mailboxes/me/rules/reorder",
		BodyFilter: requestRuleIDsEqual([]string{"r3", "r2", "r1"}),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder", "--rule-ids", `["r3","r2","r1"]`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	assertStringSliceField(t, data, "appended_rule_ids", []string{})
	assertStringSliceField(t, data, "final_rule_ids", []string{"r3", "r2", "r1"})
}

func TestRuleReorderRejectsDuplicateInput(t *testing.T) {
	f, stdout, _, _ := mailRuleReorderTestFactory(t)
	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder", "--rule-id", "r1", "--rule-ids", "r2,r1",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected duplicate validation error, got nil")
	}
	assertRuleReorderValidationError(t, err, errs.SubtypeInvalidArgument, "--rule-id/--rule-ids")
}

func TestRuleReorderRejectsMissingRuleID(t *testing.T) {
	f, stdout, _, reg := mailRuleReorderTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body:   ruleListResponse("r1", "r2"),
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder", "--rule-id", "missing",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected missing rule validation error, got nil")
	}
	assertRuleReorderValidationError(t, err, errs.SubtypeInvalidArgument, "")
}

func TestRuleReorderRejectsEmptyRuleList(t *testing.T) {
	f, stdout, _, reg := mailRuleReorderTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/rules",
		Body:   ruleListResponse(),
	})

	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder", "--rule-id", "r1",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected empty rules validation error, got nil")
	}
	assertRuleReorderValidationError(t, err, errs.SubtypeInvalidArgument, "")
}

func TestParseRuleIDsFlagSupportsJSONAndCSV(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "json", raw: `["r3", " r1 ", ""]`, want: []string{"r3", "r1"}},
		{name: "csv", raw: "r3, r1,,r2", want: []string{"r3", "r1", "r2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRuleIDsFlag(tt.raw)
			if err != nil {
				t.Fatalf("parseRuleIDsFlag() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseRuleIDsFlag() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRuleReorderDryRunShowsListAndReorder(t *testing.T) {
	f, stdout, _, _ := mailRuleReorderTestFactory(t)
	err := runMountedMailShortcut(t, MailRuleReorder, []string{
		"+rule-reorder", "--rule-ids", "r3,r1", "--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		`"method": "GET"`,
		`"url": "/open-apis/mail/v1/user_mailboxes/me/rules"`,
		`"method": "POST"`,
		`"url": "/open-apis/mail/v1/user_mailboxes/me/rules/reorder"`,
		`auto-completed-after-list`,
		`"requested_rule_ids"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q; got %s", want, out)
		}
	}
}

func ruleListResponse(ids ...string) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]interface{}{"id": id})
	}
	return map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"items": items,
		},
	}
}

func assertRuleReorderValidationError(t *testing.T, err error, wantSubtype errs.Subtype, wantParam string) {
	t.Helper()

	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed problem", err, err)
	}
	if problem.Category != errs.CategoryValidation {
		t.Fatalf("problem category = %q, want %q", problem.Category, errs.CategoryValidation)
	}
	if problem.Subtype != wantSubtype {
		t.Fatalf("problem subtype = %q, want %q", problem.Subtype, wantSubtype)
	}
	if wantParam == "" {
		return
	}

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T, want *errs.ValidationError", err)
	}
	for _, param := range validationErr.Params {
		if param.Name == wantParam {
			return
		}
	}
	t.Fatalf("validation params = %#v, want param %q", validationErr.Params, wantParam)
}

func requestRuleIDsEqual(want []string) func([]byte) bool {
	return func(body []byte) bool {
		var payload struct {
			RuleIDs []string `json:"rule_ids"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return false
		}
		return reflect.DeepEqual(payload.RuleIDs, want)
	}
}

func assertStringSliceField(t *testing.T, data map[string]interface{}, field string, want []string) {
	t.Helper()
	raw, ok := data[field].([]interface{})
	if !ok {
		t.Fatalf("%s has type %T, want []interface{}", field, data[field])
	}
	if len(raw) != len(want) {
		t.Fatalf("%s length = %d, want %d; value=%v", field, len(raw), len(want), raw)
	}
	got := make([]string, 0, len(raw))
	for _, item := range raw {
		got = append(got, item.(string))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", field, got, want)
	}
}
