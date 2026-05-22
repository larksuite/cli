// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/internal/output"
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
			return output.ErrValidation("--minute-token is required")
		}
		if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		if strings.TrimSpace(runtime.Str("topic")) == "" {
			return output.ErrValidation("--topic is required")
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

		_, err := runtime.CallAPI(http.MethodPatch,
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
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Code != minutesUpdateNoEditPermissionCode {
		return err
	}

	return &output.ExitError{
		Code: output.ExitAPI,
		Detail: &output.ErrDetail{
			Type:    "no_edit_permission",
			Code:    minutesUpdateNoEditPermissionCode,
			Message: fmt.Sprintf("No edit permission for minute %q: cannot update the title.", minuteToken),
			Hint:    "Ask the minute owner for minute edit permission",
			Detail:  exitErr.Detail.Detail,
		},
		Err: err,
	}
}
