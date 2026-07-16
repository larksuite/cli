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

// MinutesApplyPermission applies for view or edit permission on a minute.
var MinutesApplyPermission = common.Shortcut{
	Service:     "minutes",
	Command:     "+apply-permission",
	Description: "Apply for view or edit permission on a minute",
	Risk:        "write",
	Scopes:      []string{"minutes:permission:apply"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
		{Name: "perm", Desc: "permission to apply for", Required: true, Enum: []string{"view", "edit"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		if minuteToken == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		perm := strings.TrimSpace(runtime.Str("perm"))
		if perm == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--perm is required").WithParam("--perm")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		return common.NewDryRunAPI().
			POST(minutesApplyPermissionPath(minuteToken)).
			Body(minutesApplyPermissionBody(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		perm := strings.TrimSpace(runtime.Str("perm"))

		_, err := runtime.CallAPITyped(http.MethodPost, minutesApplyPermissionPath(minuteToken), nil, map[string]interface{}{"perm": perm})
		if err != nil {
			return err
		}

		runtime.OutFormat(map[string]interface{}{
			"minute_token": minuteToken,
			"perm":         perm,
		}, nil, nil)
		return nil
	},
}

func minutesApplyPermissionPath(minuteToken string) string {
	return fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/permissions/apply", validate.EncodePathSegment(minuteToken))
}

func minutesApplyPermissionBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"perm": strings.TrimSpace(runtime.Str("perm")),
	}
}
