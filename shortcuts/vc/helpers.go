// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingQueryUserScope = "vc:meeting.meetingevent:read"
	meetingQueryBotScope  = "vc:meeting.bot.join:write"
)

// meetingQueryAnyScopes are the scopes accepted by the VC meeting query
// commands (+meeting-list-active, +meeting-events). UAT recommends
// vc:meeting.meetingevent:read and TAT recommends vc:meeting.bot.join:write,
// but both identities accept either scope for compatibility.
//
// The shortcut framework's Scopes/UserScopes/BotScopes preflight is AND, so
// it cannot express "any of these". Those commands therefore leave the
// unconditional scope fields empty and call checkMeetingQueryAnyScope from
// Validate instead.
var meetingQueryAnyScopes = []string{
	meetingQueryUserScope,
	meetingQueryBotScope,
}

func checkMeetingQueryAnyScope(ctx context.Context, runtime *common.RuntimeContext) error {
	if runtime == nil || runtime.Config == nil {
		return nil
	}
	if runtime.Factory == nil || runtime.Factory.Credential == nil {
		return nil
	}
	// Mirror the framework's best-effort local preflight: rely on the resolved
	// token scopes when available, but if scope state cannot be determined
	// locally, skip the pre-check and let the API remain the source of truth.
	result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
	if err != nil {
		return nil //nolint:nilerr // intentional: fall back to remote authorization
	}
	if result == nil || result.Scopes == "" {
		return nil
	}
	if hasAnyGrantedScope(result.Scopes, meetingQueryAnyScopes) {
		return nil
	}
	required := meetingQueryUserScope
	if runtime.As().IsBot() {
		required = meetingQueryBotScope
	}
	return newMeetingQueryPermissionError(runtime, required)
}

func newMeetingQueryPermissionError(runtime *common.RuntimeContext, required string) error {
	permissionErr := errs.NewPermissionError(
		errs.SubtypeMissingScope,
		"missing required scope(s): %s",
		required,
	).
		WithMissingScopes(required).
		WithIdentity(string(runtime.As()))
	if runtime.As().IsBot() {
		consoleURL := registry.BuildConsoleScopeURL(runtime.Config.Brand, runtime.Config.AppID, required)
		return permissionErr.
			WithConsoleURL(consoleURL).
			WithHint("the app developer must apply for scope %s at the developer console: %s", required, consoleURL)
	}
	return permissionErr.WithHint("run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", required)
}

func hasAnyGrantedScope(granted string, candidates []string) bool {
	for _, scope := range candidates {
		if len(auth.MissingScopes(granted, []string{scope})) == 0 {
			return true
		}
	}
	return false
}
