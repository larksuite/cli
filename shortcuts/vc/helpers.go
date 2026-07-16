// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	meetingQueryUserScope = "vc:meeting.meetingevent:read"
	meetingQueryBotScope  = "vc:meeting.bot.join:write"
)

func normalizeMeetingQueryPermissionError(runtime *common.RuntimeContext, err error) error {
	if runtime == nil {
		return err
	}
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) || permissionErr == nil {
		return err
	}

	switch {
	case runtime.As() == core.AsUser && permissionErr.Code == output.LarkErrUserScopeInsufficient:
		permissionErr.Message = "access denied for user identity; recommended scope: " + meetingQueryUserScope
		permissionErr.WithHint("for user identity, run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", meetingQueryUserScope)
		permissionErr.WithMissingScopes(meetingQueryUserScope)
		return err
	case runtime.As() == core.AsBot && permissionErr.Code == output.LarkErrAppScopeNotEnabled:
		permissionErr.Message = "access denied for bot identity; recommended scope: " + meetingQueryBotScope
		permissionErr.WithHint("ask the app developer to enable scope %s", meetingQueryBotScope)
		permissionErr.WithMissingScopes(meetingQueryBotScope)
		if runtime.Config != nil {
			consoleURL := registry.BuildConsoleScopeURL(runtime.Config.Brand, runtime.Config.AppID, meetingQueryBotScope)
			if consoleURL != "" {
				permissionErr.WithConsoleURL(consoleURL)
			}
		}
		return err
	default:
		return err
	}
}
