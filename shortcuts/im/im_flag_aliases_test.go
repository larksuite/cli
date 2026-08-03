// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestChatMessagesListAliasesMatchCanonicalRequest(t *testing.T) {
	aliasRT := newMsgListTestRT(t, map[string]string{
		"chat-id":    "oc_test",
		"start-time": "2026-07-27 00:00:00 +08:00",
		"end-time":   "1785254400",
		"sort-order": "asc",
		"limit":      "25",
	})
	canonicalRT := newMsgListTestRT(t, map[string]string{
		"chat-id":   "oc_test",
		"start":     "2026-07-27 00:00:00 +08:00",
		"end":       "1785254400",
		"order":     "asc",
		"page-size": "25",
	})

	aliasParams, err := buildChatMessageListRequest(aliasRT, "oc_test")
	if err != nil {
		t.Fatal(err)
	}
	canonicalParams, err := buildChatMessageListRequest(canonicalRT, "oc_test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aliasParams, canonicalParams) {
		t.Fatalf("alias request = %#v, canonical request = %#v", aliasParams, canonicalParams)
	}
}

func TestChatMessagesListCanonicalFlagsWinOverAliases(t *testing.T) {
	bothRT := newMsgListTestRT(t, map[string]string{
		"chat-id":    "oc_test",
		"start":      "2026-07-27 00:00:00 +08:00",
		"start-time": "2026-07-26 00:00:00 +08:00",
		"end":        "2026-07-28 00:00:00 +08:00",
		"end-time":   "2026-07-29 00:00:00 +08:00",
		"order":      "asc",
		"sort-order": "desc",
		"page-size":  "25",
		"limit":      "30",
	})
	canonicalRT := newMsgListTestRT(t, map[string]string{
		"chat-id":   "oc_test",
		"start":     "2026-07-27 00:00:00 +08:00",
		"end":       "2026-07-28 00:00:00 +08:00",
		"order":     "asc",
		"page-size": "25",
	})

	got, err := buildChatMessageListRequest(bothRT, "oc_test")
	if err != nil {
		t.Fatal(err)
	}
	want, err := buildChatMessageListRequest(canonicalRT, "oc_test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("both-set request = %#v, canonical request = %#v", got, want)
	}
}

func TestChatMessagesListLimitAliasKeepsPageSizeValidation(t *testing.T) {
	rt := newMsgListTestRT(t, map[string]string{"limit": "100"})
	_, err := buildChatMessageListRequest(rt, "oc_test")
	assertAliasValidationError(t, err, "--limit", "invalid --limit 100: must be between 1 and 50")
}

func TestThreadsMessagesListThreadIDAlias(t *testing.T) {
	aliasRT := newThreadsTestRT(t, map[string]string{"thread-id": "omt_alias"})
	canonicalRT := newThreadsTestRT(t, map[string]string{"thread": "omt_alias"})

	if err := ImThreadsMessagesList.Validate(context.Background(), aliasRT); err != nil {
		t.Fatalf("alias validation error = %v", err)
	}
	if got, want := mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), aliasRT)), mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), canonicalRT)); got != want {
		t.Fatalf("alias dry-run differs from canonical:\nalias=%s\ncanonical=%s", got, want)
	}
}

func TestThreadsMessagesListCanonicalThreadWins(t *testing.T) {
	rt := newThreadsTestRT(t, map[string]string{
		"thread":    "omt_canonical",
		"thread-id": "omt_alias",
	})
	got, param := resolveThreadsInput(rt)
	if got != "omt_canonical" {
		t.Fatalf("resolveThreadsInput() = %q, want omt_canonical", got)
	}
	if param != "--thread" {
		t.Fatalf("resolveThreadsInput() param = %q, want --thread (canonical wins)", param)
	}
}

func TestThreadsMessagesListStillRequiresThreadInput(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{}, nil)
	err := ImThreadsMessagesList.Validate(context.Background(), rt)
	assertAliasValidationError(t, err, "--thread", "--thread is required (om_xxx or omt_xxx)")
}

func TestMessagesMGetMessageIDAlias(t *testing.T) {
	aliasRT := newTestRuntimeContext(t, map[string]string{"message-id": "om_alias"}, nil)
	canonicalRT := newTestRuntimeContext(t, map[string]string{"message-ids": "om_alias"}, nil)

	if err := ImMessagesMGet.Validate(context.Background(), aliasRT); err != nil {
		t.Fatalf("alias validation error = %v", err)
	}
	if got, want := mustMarshalDryRun(t, ImMessagesMGet.DryRun(context.Background(), aliasRT)), mustMarshalDryRun(t, ImMessagesMGet.DryRun(context.Background(), canonicalRT)); got != want {
		t.Fatalf("alias dry-run differs from canonical:\nalias=%s\ncanonical=%s", got, want)
	}
}

func TestMessagesMGetCanonicalMessageIDsWin(t *testing.T) {
	rt := newTestRuntimeContext(t, map[string]string{
		"message-ids": "om_canonical",
		"message-id":  "om_alias",
	}, nil)
	if got := resolveMessageIDsInput(rt); got != "om_canonical" {
		t.Fatalf("resolveMessageIDsInput() = %q, want om_canonical", got)
	}
}

func TestMessagesMGetStillRequiresMessageIDs(t *testing.T) {
	rt := newTestRuntimeContext(t, map[string]string{}, nil)
	err := ImMessagesMGet.Validate(context.Background(), rt)
	assertAliasValidationError(t, err, "--message-ids", "--message-ids is required (comma-separated om_xxx)")
}

func TestMessagesSearchAliasesMatchCanonicalRequest(t *testing.T) {
	aliasRT := newMessagesSearchTestRuntimeContext(t, map[string]string{
		"keyword": "project",
		"limit":   "30",
	}, nil)
	canonicalRT := newMessagesSearchTestRuntimeContext(t, map[string]string{
		"query":     "project",
		"page-size": "30",
	}, nil)

	aliasReq, err := buildMessagesSearchRequest(aliasRT)
	if err != nil {
		t.Fatal(err)
	}
	canonicalReq, err := buildMessagesSearchRequest(canonicalRT)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aliasReq, canonicalReq) {
		t.Fatalf("alias request = %#v, canonical request = %#v", aliasReq, canonicalReq)
	}
}

func TestMessagesSearchCanonicalFlagsWinOverAliases(t *testing.T) {
	rt := newMessagesSearchTestRuntimeContext(t, map[string]string{
		"query":     "canonical",
		"keyword":   "alias",
		"page-size": "25",
		"limit":     "30",
	}, nil)
	req, err := buildMessagesSearchRequest(rt)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.body["query"]; got != "canonical" {
		t.Fatalf("query = %#v, want canonical", got)
	}
	if got := req.params["page_size"][0]; got != "25" {
		t.Fatalf("page_size = %q, want 25", got)
	}
}

func TestMessagesSearchLimitAliasKeepsPageSizeValidation(t *testing.T) {
	rt := newMessagesSearchTestRuntimeContext(t, map[string]string{"limit": "100"}, nil)
	_, err := buildMessagesSearchRequest(rt)
	assertAliasValidationError(t, err, "--limit", "invalid --limit 100: must be between 1 and 50")
}

func TestIMFlagAliasesAreHiddenAndTypeCompatible(t *testing.T) {
	tests := []struct {
		shortcut  *common.Shortcut
		alias     string
		canonical string
	}{
		{&ImChatMessageList, "start-time", "start"},
		{&ImChatMessageList, "end-time", "end"},
		{&ImChatMessageList, "sort-order", "order"},
		{&ImChatMessageList, "limit", "page-size"},
		{&ImThreadsMessagesList, "thread-id", "thread"},
		{&ImMessagesMGet, "message-id", "message-ids"},
		{&ImMessagesSearch, "keyword", "query"},
		{&ImMessagesSearch, "limit", "page-size"},
	}

	for _, tt := range tests {
		t.Run(tt.shortcut.Command+"/"+tt.alias, func(t *testing.T) {
			alias := findIMFlag(t, tt.shortcut, tt.alias)
			canonical := findIMFlag(t, tt.shortcut, tt.canonical)
			if !alias.Hidden {
				t.Fatalf("--%s must be hidden", tt.alias)
			}
			if alias.Required {
				t.Fatalf("--%s must not use Cobra required validation", tt.alias)
			}
			if alias.Type != canonical.Type {
				t.Fatalf("--%s type = %q, --%s type = %q", tt.alias, alias.Type, tt.canonical, canonical.Type)
			}
			if len(alias.Enum) != 0 {
				// Declared enums are framework-validated before canonical-wins
				// resolution, so an inert alias value would fail the command
				// even when the canonical flag is present. Value sets for
				// aliases are enforced by validateAliasEnum in Validate.
				t.Fatalf("--%s (hidden alias) must not declare an Enum, got %v", tt.alias, alias.Enum)
			}
			if alias.Default != "" {
				t.Fatalf("--%s default = %q, want empty", tt.alias, alias.Default)
			}
		})
	}

	if findIMFlag(t, &ImThreadsMessagesList, "thread").Required {
		t.Fatal("--thread must use shortcut validation so --thread-id can satisfy the requirement")
	}
	if findIMFlag(t, &ImMessagesMGet, "message-ids").Required {
		t.Fatal("--message-ids must use shortcut validation so --message-id can satisfy the requirement")
	}
}

func TestExistingIMAliasesNowWriteCanonicalNotes(t *testing.T) {
	tests := []struct {
		name string
		rt   *common.RuntimeContext
		run  func(*common.RuntimeContext)
		note string
	}{
		{
			name: "chat list sort type",
			rt:   newChatListTestRuntimeContext(t, map[string]string{"sort-type": "ByActiveTimeDesc"}, nil),
			run:  func(rt *common.RuntimeContext) { _ = buildChatListParams(rt, "") },
			note: "note: --sort-type is an alias for --sort\n",
		},
		{
			name: "chat messages sort",
			rt:   newMsgListTestRT(t, map[string]string{"sort": "desc"}),
			run: func(rt *common.RuntimeContext) {
				_, _ = buildChatMessageListRequest(rt, "oc_test")
			},
			note: "note: --sort is an alias for --order\n",
		},
		{
			name: "chat search sort by",
			rt:   newSearchTestRT(t, map[string]string{"query": "team", "sort-by": "create_time_desc"}),
			run:  func(rt *common.RuntimeContext) { _ = buildSearchChatBody(rt) },
			note: "note: --sort-by is an alias for --sort\n",
		},
		{
			name: "thread messages sort",
			rt:   newThreadsTestRT(t, map[string]string{"sort": "desc"}),
			run:  func(rt *common.RuntimeContext) { _ = resolveThreadsOrder(rt) },
			note: "note: --sort is an alias for --order\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(tt.rt)
			if got := tt.rt.IO().ErrOut.(*bytes.Buffer).String(); got != tt.note {
				t.Fatalf("stderr = %q, want %q", got, tt.note)
			}
			if got := tt.rt.IO().Out.(*bytes.Buffer).String(); got != "" {
				t.Fatalf("alias note leaked to stdout: %q", got)
			}
		})
	}
}

func findIMFlag(t *testing.T, shortcut *common.Shortcut, name string) *common.Flag {
	t.Helper()
	for i := range shortcut.Flags {
		if shortcut.Flags[i].Name == name {
			return &shortcut.Flags[i]
		}
	}
	t.Fatalf("%s is missing --%s", shortcut.Command, name)
	return nil
}

func assertAliasValidationError(t *testing.T, err error, wantParam, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error is not typed: %T %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error is not *errs.ValidationError: %T %v", err, err)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("param = %q, want %q", validationErr.Param, wantParam)
	}
	if !strings.Contains(err.Error(), wantMessage) {
		t.Fatalf("error = %q, want substring %q", err, wantMessage)
	}
}

// --- review regressions: error attribution and inert-alias enum handling ---

// Alias-supplied values must attribute failures to the flag the caller
// actually typed, not to the canonical flag it maps to.
func TestChatMessagesListAliasErrorsNameTypedFlag(t *testing.T) {
	rt := newTestRuntimeContext(t, map[string]string{"start-time": "bad-time"}, nil)
	_, err := buildChatMessageListRequest(rt, "oc_x")
	assertAliasValidationError(t, err, "--start-time", "--start-time: cannot parse time")

	rt = newTestRuntimeContext(t, map[string]string{"end-time": "also-bad"}, nil)
	_, err = buildChatMessageListRequest(rt, "oc_x")
	assertAliasValidationError(t, err, "--end-time", "--end-time: cannot parse time")
}

func TestThreadsMessagesListThreadIDAliasErrorNamesTypedFlag(t *testing.T) {
	rt := newThreadsTestRT(t, map[string]string{"thread-id": "not-a-thread"})
	err := ImThreadsMessagesList.Validate(context.Background(), rt)
	assertAliasValidationError(t, err, "--thread-id", `invalid --thread-id "not-a-thread"`)
}

func TestMessagesMGetMessageIDAliasErrorNamesTypedFlag(t *testing.T) {
	rt := newTestRuntimeContext(t, map[string]string{"message-id": "not-om"}, nil)
	err := ImMessagesMGet.Validate(context.Background(), rt)
	assertAliasValidationError(t, err, "--message-id", `invalid message ID "not-om"`)
}

// A hidden alias with an invalid value must be ignored entirely when the
// canonical flag is present (canonical wins), and rejected under its own
// name when it is the flag in effect.
func TestValidateAliasEnum(t *testing.T) {
	rt := newTestRuntimeContext(t, map[string]string{"order": "asc", "sort-order": "unexpected"}, nil)
	if err := validateAliasEnum(rt, "sort-order", "order", "asc", "desc"); err != nil {
		t.Fatalf("inert alias value must not fail the command: %v", err)
	}
	params, err := buildChatMessageListRequest(rt, "oc_x")
	if err != nil {
		t.Fatalf("buildChatMessageListRequest() error = %v", err)
	}
	if got := params["sort_type"][0]; got != "ByCreateTimeAsc" {
		t.Fatalf("sort_type = %q, want ByCreateTimeAsc (canonical --order asc wins)", got)
	}

	rt = newTestRuntimeContext(t, map[string]string{"sort-order": "unexpected"}, nil)
	err = validateAliasEnum(rt, "sort-order", "order", "asc", "desc")
	assertAliasValidationError(t, err, "--sort-order", `invalid value "unexpected" for --sort-order, allowed: asc, desc`)

	rt = newTestRuntimeContext(t, map[string]string{"sort-order": "desc"}, nil)
	if err := validateAliasEnum(rt, "sort-order", "order", "asc", "desc"); err != nil {
		t.Fatalf("valid alias value must pass: %v", err)
	}
}
