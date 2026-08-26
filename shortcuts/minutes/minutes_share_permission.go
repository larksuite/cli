// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// MinutesSharePermission shares a minute permission with all meeting participants.
var MinutesSharePermission = common.Shortcut{
	Service:     "minutes",
	Command:     "+share-permission",
	Description: "Share a minute with all meeting participants",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		if minuteToken == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		return common.NewDryRunAPI().
			POST(minutesSharePermissionPath(minuteToken))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))

		_, err := runtime.CallAPITyped(http.MethodPost, minutesSharePermissionPath(minuteToken), nil, nil)
		if err != nil {
			return err
		}

		runtime.OutFormat(map[string]any{
			"minute_token": minuteToken,
			"shared":       true,
		}, nil, nil)
		return nil
	},
}

func minutesSharePermissionPath(minuteToken string) string {
	return fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/permissions/share", validate.EncodePathSegment(minuteToken))
}
