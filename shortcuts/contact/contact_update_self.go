// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	selfAvatarUserInfoPath = "/open-apis/authen/v1/user_info"
	selfAvatarEmployeePath = "/open-apis/directory/v1/employees/"
	selfAvatarUpdateScope  = "directory:employee.update:write"
)

type updateSelfOutput struct {
	EmployeeID     string   `json:"employee_id"`
	EmployeeIDType string   `json:"employee_id_type"`
	UpdatedFields  []string `json:"updated_fields"`
}

type selfAvatarPatch struct {
	Employee selfAvatarEmployee `json:"employee"`
}
type selfAvatarEmployee struct {
	AvatarKey string `json:"avatar_key"`
}
type selfAvatarUserInfo struct {
	OpenID string `json:"open_id"`
}

// ContactUpdateSelf resolves the current user and updates only their avatar.
var ContactUpdateSelf = common.Shortcut{
	Service:               "contact",
	Command:               "+update-self",
	Description:           "Update your own avatar using ordinary-user permissions",
	Risk:                  "write",
	Scopes:                []string{},
	AuthTypes:             []string{"user"},
	ConditionalUserScopes: []string{selfAvatarUpdateScope, selfAvatarImageResourceScope},
	Flags: []common.Flag{
		{Name: "avatar-key", Desc: "existing image key uploaded with image_type=avatar; cannot be combined with --avatar-file"},
		{Name: "avatar-file", Desc: "local avatar image path (not a URL), at most 10 MiB; cannot be combined with --avatar-key"},
	},
	Validate: validateUpdateSelf,
	DryRun:   previewUpdateSelf,
	Execute:  executeUpdateSelf,
}

func validateUpdateSelf(_ context.Context, runtime *common.RuntimeContext) error {
	key, file := runtime.Str("avatar-key"), runtime.Str("avatar-file")
	hasKey, hasFile := runtime.Changed("avatar-key"), runtime.Changed("avatar-file")
	if hasKey && hasFile {
		return common.ValidationErrorf("--avatar-key and --avatar-file cannot be combined").WithParam("--avatar-file")
	}
	if !hasKey && !hasFile {
		return common.ValidationErrorf("provide --avatar-file or --avatar-key to update your own avatar")
	}
	if hasKey && strings.TrimSpace(key) == "" {
		return common.ValidationErrorf("--avatar-key must be a non-empty image key").WithParam("--avatar-key")
	}
	if hasFile && (strings.TrimSpace(file) == "" || strings.Contains(file, "://")) {
		return common.ValidationErrorf("--avatar-file must be a non-empty local image path; URLs are not accepted").WithParam("--avatar-file")
	}
	return nil
}

func executeUpdateSelf(_ context.Context, runtime *common.RuntimeContext) error {
	key, file := runtime.Str("avatar-key"), runtime.Str("avatar-file")
	// Check only at execution so dry-run never resolves credentials.
	if err := ensureSelfAvatarScope(runtime, selfAvatarUpdateScope, "directory:employee:write"); err != nil {
		return err
	}
	if file != "" {
		if err := ensureSelfAvatarScope(runtime, selfAvatarImageResourceScope, "im:resource:upload"); err != nil {
			return err
		}
	}
	user, err := callSelfAvatarAPI[selfAvatarUserInfo](runtime, "GET", selfAvatarUserInfoPath, nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(user.OpenID) == "" {
		return contactInvalidResponseError("user_info returned no current-user open_id; avatar update was not sent")
	}
	if file != "" {
		key, err = uploadSelfAvatar(runtime, file)
		if err != nil {
			return err
		}
	}
	if _, err := runtime.CallAPITyped("PATCH", selfAvatarEmployeePath+url.PathEscape(user.OpenID),
		map[string]interface{}{"employee_id_type": "open_id", "user_auth_type": "user"}, selfAvatarPatchBody(key)); err != nil {
		return err
	}
	runtime.Out(updateSelfOutput{EmployeeID: user.OpenID, EmployeeIDType: "open_id", UpdatedFields: []string{"avatar_key"}}, nil)
	return nil
}

// API scope alternatives are OR conditions; EnsureScopes treats its list as AND.
// Keep the recommended scope's native recovery when neither grant is available.
func ensureSelfAvatarScope(runtime *common.RuntimeContext, recommended, alternative string) error {
	err := runtime.EnsureScopes([]string{recommended})
	var permission *errs.PermissionError
	if !errors.As(err, &permission) || permission.Subtype != errs.SubtypeMissingScope {
		return err
	}
	alternativeErr := runtime.EnsureScopes([]string{alternative})
	if alternativeErr == nil {
		return nil
	}
	if errors.As(alternativeErr, &permission) && permission.Subtype == errs.SubtypeMissingScope {
		return err
	}
	return alternativeErr
}

// Decode only the fields consumed by this workflow; API and transport failures
// retain the native runtime's typed error metadata.
func callSelfAvatarAPI[T any](runtime *common.RuntimeContext, method, path string, body any) (T, error) {
	var value T
	data, err := runtime.CallAPITyped(method, path, nil, body)
	if err != nil {
		return value, err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return value, contactInvalidResponseError("encode avatar API response data").WithCause(err)
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return value, contactInvalidResponseError("decode avatar API response data").WithCause(err)
	}
	return value, nil
}

func selfAvatarPatchBody(key string) selfAvatarPatch {
	return selfAvatarPatch{Employee: selfAvatarEmployee{AvatarKey: key}}
}

func previewUpdateSelf(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	preview := common.NewDryRunAPI().GET(selfAvatarUserInfoPath).
		Desc("Resolve the current credential owner's open_id; no request is made during preview.")
	key := runtime.Str("avatar-key")
	if runtime.Changed("avatar-file") {
		preview.POST(selfAvatarImagePath).Body(struct {
			ImageType string `json:"image_type"`
			Image     string `json:"image"`
		}{"avatar", "<file bytes from " + runtime.Str("avatar-file") + ">"}).
			Desc("Multipart image upload at execution; the preview contains no file contents.")
		key = "<image_key from avatar upload>"
	}
	return preview.PATCH(selfAvatarEmployeePath+":employee_id").Set("employee_id", "<current_user_open_id>").
		Params(map[string]interface{}{"employee_id_type": "open_id", "user_auth_type": "user"}).Body(selfAvatarPatchBody(key)).
		Desc("The path ID comes from user_info; only the current user's avatar is updated.")
}
