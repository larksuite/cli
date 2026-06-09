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

const minutesUpdateNoEditPermissionCode = 2091005

// MinutesUpdate updates the title (topic) of a minute.
var MinutesUpdate = common.Shortcut{
	Service:     "minutes",
	Command:     "+update",
	Description: "Update a minute's title",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes:update"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "minute token", Required: true},
		{Name: "topic", Desc: "new minute title", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		if minuteToken == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--minute-token is required").WithParam("--minute-token")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--minute-token")
		}
		if strings.TrimSpace(runtime.Str("topic")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--topic is required").WithParam("--topic")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		return common.NewDryRunAPI().
			PATCH(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(minuteToken))).
			Body(map[string]interface{}{"topic": runtime.Str("topic")})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		minuteToken := strings.TrimSpace(runtime.Str("minute-token"))
		topic := runtime.Str("topic")

		body := map[string]interface{}{
			"topic": topic,
		}

		_, err := runtime.CallAPITyped(http.MethodPatch,
			fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(minuteToken)),
			nil, body)
		if err != nil {
			return minutesUpdateError(err, minuteToken)
		}

		outData := map[string]interface{}{
			"minute_token": minuteToken,
			"topic":        topic,
		}

		runtime.OutFormat(outData, nil, nil)
		return nil
	},
}

func minutesUpdateError(err error, minuteToken string) error {
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != minutesUpdateNoEditPermissionCode {
		return err
	}
	p.Message = fmt.Sprintf("No edit permission for minute %q: cannot update the title.", minuteToken)
	p.Hint = "Ask the minute owner for minute edit permission"
	return err
}
