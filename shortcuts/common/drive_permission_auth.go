// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
)

const (
	// DrivePermissionMemberAuthScope is required by the Drive permission auth API.
	DrivePermissionMemberAuthScope = "docs:permission.member:auth"

	drivePermissionResourceTypeFile = "file"
	drivePermissionActionExport     = "export"
)

type drivePermissionMemberAuthResponse struct {
	AuthResult bool
}

// CheckDriveFileExportPermission checks whether the runtime's current identity
// can export one Drive file. It never switches identity and does not treat a
// malformed success response as a denial.
func CheckDriveFileExportPermission(runtime *RuntimeContext, token string) (bool, error) {
	data, err := runtime.CallAPITyped(
		http.MethodGet,
		drivePermissionMemberAuthPath(token),
		map[string]interface{}{
			"type":   drivePermissionResourceTypeFile,
			"action": drivePermissionActionExport,
		},
		nil,
	)
	if err != nil {
		return false, err
	}

	response, err := projectDrivePermissionMemberAuthResponse(data)
	if err != nil {
		return false, err
	}
	return response.AuthResult, nil
}

func projectDrivePermissionMemberAuthResponse(data map[string]interface{}) (drivePermissionMemberAuthResponse, error) {
	authResult, ok := data["auth_result"].(bool)
	if !ok {
		return drivePermissionMemberAuthResponse{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"Drive permission auth response is missing a boolean auth_result",
		)
	}
	return drivePermissionMemberAuthResponse{AuthResult: authResult}, nil
}

func drivePermissionMemberAuthPath(token string) string {
	return fmt.Sprintf(
		"/open-apis/drive/v1/permissions/%s/members/auth",
		validate.EncodePathSegment(token),
	)
}

// AddDriveFileExportPermissionDryRun appends the permission check used by the
// real download flow so dry-run output preserves request order and parameters.
func AddDriveFileExportPermissionDryRun(plan *DryRunAPI, token, desc string) *DryRunAPI {
	return plan.
		GET(drivePermissionMemberAuthPath(token)).
		Params(map[string]interface{}{
			"type":   drivePermissionResourceTypeFile,
			"action": drivePermissionActionExport,
		}).
		Desc(desc)
}
