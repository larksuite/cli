// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/core"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/output"
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
	var exit *output.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected output.ExitError, got %T: %v", err, err)
	}
	if exit.Code != output.ExitValidation {
		t.Errorf("ExitCode = %d, want ExitValidation (%d)", exit.Code, output.ExitValidation)
	}
	if exit.Detail == nil {
		t.Fatal("expected Detail with hint")
	}
	wantURL := "https://open.feishu.cn/app/cli_XXXXXXXXXXXXXXXX/event"
	if !strings.Contains(exit.Detail.Hint, wantURL) {
		t.Errorf("hint missing subscription URL %q\ngot: %s", wantURL, exit.Detail.Hint)
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
	var exit *output.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("expected output.ExitError, got %T: %v", err, err)
	}
	if exit.Code != output.ExitAuth {
		t.Errorf("ExitCode = %d, want ExitAuth (%d)", exit.Code, output.ExitAuth)
	}
	if exit.Detail == nil {
		t.Fatal("expected Detail with hint, got nil Detail")
	}
	hint := exit.Detail.Hint
	wantSubstrings := []string{
		"https://open.feishu.cn/app/cli_x/auth?q=",
		"im:message.group_at_msg",
		"token_type=tenant",
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
