// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingQueryUserScope = "vc:meeting.meetingevent:read"
	meetingQueryBotScope  = "vc:meeting.bot.join:write"
)

// meetingQueryAnyScopes are the compatible alternatives reported by the VC
// API. The local preflight only checks the user recommendation because UAT,
// unlike TAT, exposes granted scope metadata.
var meetingQueryAnyScopes = []string{
	meetingQueryUserScope,
	meetingQueryBotScope,
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
