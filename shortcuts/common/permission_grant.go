// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
)

const (
	PermissionGrantGranted  = "granted"
	PermissionGrantSkipped  = "skipped"
	PermissionGrantFailed   = "failed"
	permissionGrantPerm     = "full_access"
	permissionGrantPermHint = "可管理权限"
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
			fmt.Sprintf("The operation did not return a permission target (missing token/type), so current user %s was not granted. You can retry later or continue using bot identity.", permissionGrantPermMessage()),
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
			fmt.Sprintf("Resource was created with bot identity, but no current CLI user open_id is configured, so current user %s was not granted. You can retry later or continue using bot identity.", permissionGrantPermMessage()),
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
			fmt.Sprintf("Resource was created, but granting current user %s failed: %s. You can retry later or continue using bot identity.", permissionGrantPermMessage(), compactPermissionGrantError(err)),
		)
	}

	return buildPermissionGrantResult(
		PermissionGrantGranted,
		userOpenID,
		fmt.Sprintf("Granted the current CLI user %s on the new %s.", permissionGrantPermMessage(), permissionTargetLabel(resourceType)),
	)
}

func buildPermissionGrantResult(status, userOpenID, message string) map[string]interface{} {
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

func permissionGrantPermMessage() string {
	return permissionGrantPerm + " (" + permissionGrantPermHint + ")"
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
	case "slides":
		return "presentation"
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
