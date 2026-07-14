// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
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
		mapped := *permissionErr
		mapped.Cause = err
		return mapped.WithHint("for user identity, run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", meetingQueryUserScope)
	case runtime.As() == core.AsBot && permissionErr.Code == output.LarkErrAppScopeNotEnabled:
		mapped := *permissionErr
		mapped.Cause = err
		mapped.ConsoleURL = ""
		return mapped.WithHint("ask the app developer to enable scope %s", meetingQueryBotScope)
	default:
		return err
	}
}
