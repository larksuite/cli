// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

var BaseAdvpermShareGet = common.Shortcut{
	Service:     "base",
	Command:     "+advperm-share-get",
	Description: "Read sharing authorization for an advanced-permission Base",
	Risk:        "read",
	Scopes:      []string{"base:app:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true)},
	Validate: func(_ context.Context, rt *common.RuntimeContext) error {
		return validatePermissionTarget(rt, false)
	},
	DryRun: func(_ context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(baseV3Path("bases", rt.Str("base-token"), "advperm", "share"))
	},
	Execute: func(_ context.Context, rt *common.RuntimeContext) error {
		return executePermissionRequest(rt, "GET", baseV3Path("bases", rt.Str("base-token"), "advperm", "share"), nil, nil)
	},
}

var BaseAdvpermShareUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+advperm-share-update",
	Description: "Explicitly enable or disable sharing authorization",
	Risk:        "write",
	Scopes:      []string{"base:app:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "enable-share", Type: "bool", Desc: "set true or false explicitly; use =false to disable"},
	},
	Validate: func(_ context.Context, rt *common.RuntimeContext) error {
		if err := validatePermissionTarget(rt, false); err != nil {
			return err
		}
		if !rt.Changed("enable-share") {
			return baseFlagErrorf("--enable-share must be explicitly set to true or false")
		}
		return nil
	},
	DryRun: func(_ context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().PUT(baseV3Path("bases", rt.Str("base-token"), "advperm", "share")).Body(permissionShareBody(rt))
	},
	Execute: func(_ context.Context, rt *common.RuntimeContext) error {
		return executePermissionRequest(rt, "PUT", baseV3Path("bases", rt.Str("base-token"), "advperm", "share"), nil, permissionShareBody(rt))
	},
}

var BaseAdvpermApply = common.Shortcut{
	Service:     "base",
	Command:     "+advperm-apply",
	Description: "Request advanced-permission access as the signed-in user",
	Risk:        "write",
	Scopes:      []string{"base:app:update"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "message", Desc: "reason to include in the access request"},
	},
	Tips: []string{"Submit as the requester using --as user. Success means the request was submitted, not that access was granted."},
	Validate: func(_ context.Context, rt *common.RuntimeContext) error {
		return validatePermissionTarget(rt, false)
	},
	DryRun: func(_ context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(baseV3Path("bases", rt.Str("base-token"), "advperm", "apply")).Body(permissionApplyBody(rt))
	},
	Execute: func(_ context.Context, rt *common.RuntimeContext) error {
		return executePermissionRequest(rt, "POST", baseV3Path("bases", rt.Str("base-token"), "advperm", "apply"), nil, permissionApplyBody(rt))
	},
}

func permissionShareBody(rt *common.RuntimeContext) interface{} {
	return struct {
		EnableShare bool `json:"enable_share"`
	}{rt.Bool("enable-share")}
}

func permissionApplyBody(rt *common.RuntimeContext) interface{} {
	return struct {
		Message string `json:"message,omitempty"`
	}{rt.Str("message")}
}

// These preview endpoints wrap their JSON payload inside data.data. Decode
// both object and string envelopes, preserving batch failure details.
func executePermissionRequest(rt *common.RuntimeContext, method, path string, params map[string]interface{}, body interface{}) error {
	query := make(larkcore.QueryParams)
	for key, value := range params {
		query.Set(key, fmt.Sprint(value))
	}
	resp, err := rt.DoAPI(&larkcore.ApiReq{HttpMethod: method, ApiPath: path, QueryParams: query, Body: body})
	if err != nil {
		return err
	}
	_, apiErr := rt.ClassifyAPIResponse(resp)
	data, businessErr := decodePermissionResponse(rt, resp.RawBody)
	if apiErr != nil {
		// A non-2xx response must never be treated as a successful payload.
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices && data != nil && permissionHasFailedMembers(data) {
			return rt.OutPartialFailure(data, nil)
		}
		return enrichBaseAPIErrorFromBody(apiErr, resp.RawBody, rt.APIClassifyContext())
	}
	if data != nil && permissionHasFailedMembers(data) {
		return rt.OutPartialFailure(data, nil)
	}
	if businessErr != nil {
		return businessErr
	}
	rt.Out(data, nil)
	return nil
}

func permissionHasFailedMembers(data json.RawMessage) bool {
	var payload struct {
		FailedMembers []json.RawMessage `json:"failed_members"`
	}
	return json.Unmarshal(data, &payload) == nil && len(payload.FailedMembers) > 0
}

func decodePermissionResponse(rt *common.RuntimeContext, raw json.RawMessage) (json.RawMessage, error) {
	var businessErr error
	for depth := 0; depth < 6; depth++ {
		raw = bytes.TrimSpace(raw)
		var encoded string
		if len(raw) > 0 && raw[0] == '"' {
			if err := json.Unmarshal(raw, &encoded); err != nil {
				return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid permission payload").WithCause(err)
			}
			raw = json.RawMessage(encoded)
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || raw[0] != '{' {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "permission response must be a JSON object")
		}
		var envelope struct {
			Code    *int            `json:"code"`
			Msg     string          `json:"msg"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid permission response").WithCause(err)
		}
		if envelope.Code != nil && *envelope.Code != 0 {
			message := envelope.Msg
			if message == "" {
				message = envelope.Message
			}
			// Keep the complete failing envelope so the shared Base classifier
			// can retain upstream hints, log IDs and authorization details.
			result, err := decodeBaseV3Response(raw)
			if err != nil {
				return nil, err
			}
			result["msg"] = message
			businessErr = baseRoleAPIError(rt, result, "permission operation failed")
		}
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			// A {code:0} envelope with no data is a bare acknowledgement, the
			// normal shape of a write with nothing to return. Handing back the
			// envelope itself would expose transport fields (code, msg) as the
			// business payload, and a top-level code is what this repository
			// reads as an upstream failure. An object without a code is not an
			// envelope but the payload, and is returned as-is.
			if envelope.Code != nil && businessErr == nil {
				return json.RawMessage(`{}`), nil
			}
			return raw, businessErr
		}
		raw = envelope.Data
	}
	return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "permission response has too many nested envelopes")
}
