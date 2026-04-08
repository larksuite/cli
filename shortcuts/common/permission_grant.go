// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
)

const (
	PermissionGrantGranted = "granted"
	PermissionGrantSkipped = "skipped"
	PermissionGrantFailed  = "failed"
	permissionGrantPerm    = "full_access"
)

// AutoGrantCurrentUserDrivePermission grants full_access on a newly created
// Drive resource to the current CLI user when the shortcut runs as bot.
//
// Callers should attach the returned result only when it is non-nil.
func AutoGrantCurrentUserDrivePermission(runtime *RuntimeContext, token, resourceType string) map[string]interface{} {
	if runtime == nil || !runtime.IsBot() {
		return nil
	}

	token = strings.TrimSpace(token)
	resourceType = strings.TrimSpace(resourceType)
	if token == "" || resourceType == "" {
		return buildPermissionGrantResult(
			PermissionGrantSkipped,
			"",
			"",
			"The operation did not return a permission target (missing token/type), so current user permission was not granted. You can retry later or continue using bot identity.",
		)
	}

	return autoGrantCurrentUserDrivePermission(runtime, token, resourceType)
}

func autoGrantCurrentUserDrivePermission(runtime *RuntimeContext, token, resourceType string) map[string]interface{} {
	userOpenID := strings.TrimSpace(runtime.UserOpenId())
	if userOpenID == "" {
		return buildPermissionGrantResult(
			PermissionGrantSkipped,
			"",
			resourceType,
			"Resource was created with bot identity, but no current CLI user open_id is configured, so user permission was not granted. You can retry later or continue using bot identity.",
		)
	}

	body := map[string]interface{}{
		"member_type": "openid",
		"member_id":   userOpenID,
		"perm":        permissionGrantPerm,
		"type":        "user",
	}
	if permType := permissionGrantPermType(resourceType); permType != "" {
		body["perm_type"] = permType
	}

	_, err := runtime.CallAPI(
		"POST",
		fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members", validate.EncodePathSegment(token)),
		map[string]interface{}{
			"type":              resourceType,
			"need_notification": false,
		},
		body,
	)
	if err != nil {
		return buildPermissionGrantResult(
			PermissionGrantFailed,
			userOpenID,
			resourceType,
			fmt.Sprintf("Resource was created, but granting current user %s failed: %s. You can retry later or continue using bot identity.", permissionGrantPerm, compactPermissionGrantError(err)),
		)
	}

	return buildPermissionGrantResult(
		PermissionGrantGranted,
		userOpenID,
		resourceType,
		fmt.Sprintf("Granted the current CLI user %s on the new %s.", permissionGrantPerm, permissionTargetLabel(resourceType)),
	)
}

func buildPermissionGrantResult(status, userOpenID, resourceType, message string) map[string]interface{} {
	result := map[string]interface{}{
		"status":  status,
		"perm":    permissionGrantPerm,
		"message": message,
	}
	if userOpenID != "" {
		result["user_open_id"] = userOpenID
		result["member_type"] = "openid"
	}
	return result
}

func permissionGrantPermType(resourceType string) string {
	switch resourceType {
	case "wiki":
		return "container"
	default:
		return ""
	}
}

func permissionTargetLabel(resourceType string) string {
	switch resourceType {
	case "wiki":
		return "wiki node"
	case "doc", "docx":
		return "document"
	case "sheet":
		return "spreadsheet"
	case "bitable", "base":
		return "base"
	case "file":
		return "file"
	case "folder":
		return "folder"
	default:
		return "resource"
	}
}

func compactPermissionGrantError(err error) string {
	if err == nil {
		return ""
	}
	return strings.Join(strings.Fields(err.Error()), " ")
}
