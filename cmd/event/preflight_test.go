// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/core"
	eventlib "github.com/larksuite/cli/internal/event"
)

func newPreflightCtx(appID string, brand core.LarkBrand, identity core.Identity, keyDef *eventlib.KeyDefinition, appVer *appmeta.AppVersion) *preflightCtx {
	key := ""
	if keyDef != nil {
		key = keyDef.Key
	}
	return &preflightCtx{
		appID:    appID,
		brand:    brand,
		eventKey: key,
		identity: identity,
		keyDef:   keyDef,
		appVer:   appVer,
	}
}

func TestPreflightEventTypes_NilAppVer_SkipsCheck(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:                   "im.message.text",
		EventType:             "im.message.receive_v1",
		RequiredConsoleEvents: []string{"im.message.receive_v1"},
	}
	if err := preflightEventTypes(newPreflightCtx("cli_x", "feishu", "", def, nil)); err != nil {
		t.Fatalf("nil appVer must be a weak-dependency skip, got err: %v", err)
	}
}

func TestPreflightEventTypes_EmptyRequired_SkipsEvenIfEventTypeSet(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:       "im.message.message_read_v1",
		EventType: "im.message.message_read_v1",
	}
	appVer := &appmeta.AppVersion{EventTypes: []string{"im.message.receive_v1"}}
	if err := preflightEventTypes(newPreflightCtx("cli_x", "feishu", "", def, appVer)); err != nil {
		t.Fatalf("empty RequiredConsoleEvents must skip, got: %v", err)
	}
}

func TestPreflightEventTypes_AllSubscribed_Passes(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:       "im.reaction",
		EventType: "im.message.reaction.created_v1",
		RequiredConsoleEvents: []string{
			"im.message.reaction.created_v1",
			"im.message.reaction.deleted_v1",
		},
	}
	appVer := &appmeta.AppVersion{EventTypes: []string{
		"im.message.reaction.created_v1",
		"im.message.reaction.deleted_v1",
		"im.message.receive_v1",
	}}
	if err := preflightEventTypes(newPreflightCtx("cli_x", "feishu", "", def, appVer)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPreflightEventTypes_MissingBlocks(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:       "mail.receive",
		EventType: "mail.user_mailbox.event.message_received_v1",
		RequiredConsoleEvents: []string{
			"mail.user_mailbox.event.message_received_v1",
			"mail.user_mailbox.event.message_read_v1",
		},
	}
	appVer := &appmeta.AppVersion{EventTypes: []string{
		"mail.user_mailbox.event.message_received_v1",
	}}
	err := preflightEventTypes(newPreflightCtx("cli_XXXXXXXXXXXXXXXX", "feishu", "", def, appVer))
	if err == nil {
		t.Fatal("expected error for missing subscription")
	}
	if !strings.Contains(err.Error(), "mail.user_mailbox.event.message_read_v1") {
		t.Errorf("error should name the missing event type, got: %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("problem = %s/%s, want %s/%s", p.Category, p.Subtype,
			errs.CategoryValidation, errs.SubtypeFailedPrecondition)
	}
	wantURL := "https://open.feishu.cn/page/launcher?clientID=cli_XXXXXXXXXXXXXXXX&addons="
	if !strings.Contains(p.Hint, wantURL) {
		t.Errorf("hint missing scan link %q\ngot: %s", wantURL, p.Hint)
	}
}

func TestPreflightScopes_Bot_NoAppVer_SkipsCheck(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:    "im.message.text",
		Scopes: []string{"im:message", "im:message.group_at_msg"},
	}
	err := preflightScopes(nil, newPreflightCtx("cli_x", "feishu", core.AsBot, def, nil))
	if err != nil {
		t.Fatalf("bot + nil appVer should skip, got: %v", err)
	}
}

func TestPreflightScopes_Bot_AllGranted_Passes(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:    "im.message.text",
		Scopes: []string{"im:message", "im:message.group_at_msg"},
	}
	appVer := &appmeta.AppVersion{TenantScopes: []string{
		"im:message",
		"im:message.group_at_msg",
		"contact:user:readonly",
	}}
	err := preflightScopes(nil, newPreflightCtx("cli_x", "feishu", core.AsBot, def, appVer))
	if err != nil {
		t.Fatalf("all scopes granted, unexpected error: %v", err)
	}
}

func TestPreflightScopes_Bot_MissingBlocks(t *testing.T) {
	def := &eventlib.KeyDefinition{
		Key:    "im.message.text",
		Scopes: []string{"im:message", "im:message.group_at_msg"},
	}
	appVer := &appmeta.AppVersion{TenantScopes: []string{"im:message"}}
	err := preflightScopes(nil, newPreflightCtx("cli_x", "feishu", core.AsBot, def, appVer))
	if err == nil {
		t.Fatal("expected error for missing scope")
	}
	if !strings.Contains(err.Error(), "im:message.group_at_msg") {
		t.Errorf("error should name missing scope, got: %v", err)
	}
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected *errs.PermissionError, got %T: %v", err, err)
	}
	if permErr.Category != errs.CategoryAuthorization || permErr.Subtype != errs.SubtypeMissingScope {
		t.Errorf("problem = %s/%s, want %s/%s", permErr.Category, permErr.Subtype,
			errs.CategoryAuthorization, errs.SubtypeMissingScope)
	}
	wantMissing := []string{"im:message.group_at_msg"}
	if len(permErr.MissingScopes) != 1 || permErr.MissingScopes[0] != wantMissing[0] {
		t.Errorf("MissingScopes = %v, want %v", permErr.MissingScopes, wantMissing)
	}
	hint := permErr.Hint
	wantSubstrings := []string{
		"grant these scopes by scanning: ",
		"https://open.feishu.cn/page/launcher?clientID=cli_x&addons=",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q\ngot: %s", want, hint)
		}
	}
}

func TestPreflightScopes_NoRequiredScopes_SkipsCheck(t *testing.T) {
	def := &eventlib.KeyDefinition{Key: "x"}
	if err := preflightScopes(nil, newPreflightCtx("cli_x", "feishu", core.AsBot, def, nil)); err != nil {
		t.Fatalf("no required scopes means nothing to verify, got: %v", err)
	}
}

func TestPreflightEventTypes_CallbackMissing(t *testing.T) {
	pf := &preflightCtx{
		appID:               "cli_x",
		brand:               core.BrandFeishu,
		eventKey:            "test.cb",
		identity:            core.AsBot,
		subscribedCallbacks: []string{"profile.view.get"},
		keyDef: &eventlib.KeyDefinition{
			Key:                   "test.cb",
			SubscriptionType:      eventlib.SubTypeCallback,
			RequiredConsoleEvents: []string{"card.action.trigger"},
		},
	}
	err := preflightEventTypes(pf)
	if err == nil {
		t.Fatal("expected error for missing callback")
	}
	if !strings.Contains(err.Error(), "callbacks not subscribed") {
		t.Errorf("error = %q, want mention of 'callbacks not subscribed'", err.Error())
	}
	if !strings.Contains(err.Error(), "card.action.trigger") {
		t.Errorf("error should name the missing callback, got: %q", err.Error())
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("problem = %v, want validation/failed_precondition", p)
	}
}

func TestPreflightEventTypes_CallbackSkippedWhenNil(t *testing.T) {
	pf := &preflightCtx{
		appID:               "cli_x",
		brand:               core.BrandFeishu,
		eventKey:            "test.cb",
		identity:            core.AsBot,
		subscribedCallbacks: nil, // fetch 失败/拿不到 -> 弱依赖跳过
		keyDef: &eventlib.KeyDefinition{
			Key:                   "test.cb",
			SubscriptionType:      eventlib.SubTypeCallback,
			RequiredConsoleEvents: []string{"card.action.trigger"},
		},
	}
	if err := preflightEventTypes(pf); err != nil {
		t.Errorf("expected skip (nil), got %v", err)
	}
}

func TestPreflightEventTypes_CallbackEmptyReportsMissing(t *testing.T) {
	// fetched but zero callbacks subscribed (non-nil empty) is a definitive
	// console state: a required callback IS missing and must be reported,
	// not skipped as a weak dependency.
	pf := &preflightCtx{
		appID:               "cli_x",
		brand:               core.BrandFeishu,
		eventKey:            "test.cb",
		identity:            core.AsBot,
		subscribedCallbacks: []string{}, // fetched, none subscribed
		keyDef: &eventlib.KeyDefinition{
			Key:                   "test.cb",
			SubscriptionType:      eventlib.SubTypeCallback,
			RequiredConsoleEvents: []string{"card.action.trigger"},
		},
	}
	err := preflightEventTypes(pf)
	if err == nil {
		t.Fatal("expected error for missing callback when none are subscribed")
	}
	if !strings.Contains(err.Error(), "card.action.trigger") {
		t.Errorf("error should name the missing callback, got: %q", err.Error())
	}
}

func TestPreflightEventTypes_CallbackAllSubscribed_Passes(t *testing.T) {
	pf := &preflightCtx{
		appID:               "cli_x",
		brand:               core.BrandFeishu,
		eventKey:            "test.cb",
		identity:            core.AsBot,
		subscribedCallbacks: []string{"card.action.trigger", "profile.view.get"},
		keyDef: &eventlib.KeyDefinition{
			Key:                   "test.cb",
			SubscriptionType:      eventlib.SubTypeCallback,
			RequiredConsoleEvents: []string{"card.action.trigger"},
		},
	}
	if err := preflightEventTypes(pf); err != nil {
		t.Errorf("all callbacks subscribed, unexpected error: %v", err)
	}
}

func TestScopeRemediationHint_ByIdentity(t *testing.T) {
	// bot: scan-to-enable link (adds scopes to app manifest)
	bot := scopeRemediationHint(core.BrandFeishu, "cli_x", core.AsBot, []string{"im:message"})
	if !strings.Contains(bot, "/page/launcher?clientID=cli_x&addons=") {
		t.Errorf("bot hint should give the scan link, got: %s", bot)
	}
	// user: re-login (scan link cannot grant scopes to the user's own token)
	user := scopeRemediationHint(core.BrandFeishu, "cli_x", core.AsUser, []string{"im:message"})
	if !strings.Contains(user, "auth login --scope") {
		t.Errorf("user hint should direct to auth login, got: %s", user)
	}
	if strings.Contains(user, "/page/launcher") {
		t.Errorf("user hint must NOT use the scan link, got: %s", user)
	}
}
