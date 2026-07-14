// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil //nolint:nilerr // intentional: fall back to remote authorization
	}
	if result == nil || result.Scopes == "" {
		return nil
	}
	if hasAnyGrantedScope(result.Scopes, meetingQueryAnyScopes) {
		return nil
	}
	return newMeetingQueryPermissionError(runtime)
}

func newMeetingQueryPermissionError(runtime *common.RuntimeContext) error {
	permissionErr := errs.NewPermissionError(
		errs.SubtypeMissingScope,
		meetingQueryMissingScopeMessage(),
	).
		WithMissingScopes(meetingQueryAnyScopes...).
		WithIdentity(string(runtime.As()))
	return addMeetingQueryRecovery(runtime, permissionErr)
}

func normalizeMeetingQueryPermissionError(runtime *common.RuntimeContext, err error) error {
	if err == nil || runtime == nil || runtime.Config == nil {
		return err
	}
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) || permissionErr == nil {
		return err
	}
	if permissionErr.Code != output.LarkErrAppScopeNotEnabled && permissionErr.Code != output.LarkErrUserScopeInsufficient {
		return err
	}
	if !containsAllScopes(permissionErr.MissingScopes, meetingQueryAnyScopes) {
		return err
	}

	mapped := *permissionErr
	mapped.Problem.Message = meetingQueryMissingScopeMessage()
	mapped.MissingScopes = append([]string(nil), permissionErr.MissingScopes...)
	mapped.RequestedScopes = append([]string(nil), permissionErr.RequestedScopes...)
	mapped.GrantedScopes = append([]string(nil), permissionErr.GrantedScopes...)
	mapped.Cause = err
	return addMeetingQueryRecovery(runtime, &mapped)
}

func meetingQueryMissingScopeMessage() string {
	return "missing compatible meeting query scopes: either " + meetingQueryUserScope + " or " + meetingQueryBotScope + " is sufficient"
}

func addMeetingQueryRecovery(runtime *common.RuntimeContext, permissionErr *errs.PermissionError) error {
	recommended := meetingQueryUserScope
	if runtime.As().IsBot() {
		recommended = meetingQueryBotScope
	}
	permissionErr.Identity = string(runtime.As())
	permissionErr.ConsoleURL = ""
	if runtime.As().IsBot() {
		consoleURL := registry.BuildConsoleScopeURL(runtime.Config.Brand, runtime.Config.AppID, recommended)
		return permissionErr.
			WithConsoleURL(consoleURL).
			WithHint("either compatible scope is sufficient; for bot identity, the app developer should apply for the recommended scope %s at the developer console: %s", recommended, consoleURL)
	}
	return permissionErr.WithHint("either compatible scope is sufficient; for user identity, run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", recommended)
}

func containsAllScopes(granted []string, required []string) bool {
	for _, requiredScope := range required {
		found := false
		for _, grantedScope := range granted {
			if grantedScope == requiredScope {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func hasAnyGrantedScope(granted string, candidates []string) bool {
	for _, scope := range candidates {
		if len(auth.MissingScopes(granted, []string{scope})) == 0 {
			return true
		}
	}
	return false
}
