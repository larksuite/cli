// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/registry"
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
			"No permission target (missing token or type) returned by the operation.",
		)
	}

	return autoGrantCurrentUserDrivePermission(runtime, token, resourceType)
}

func autoGrantCurrentUserDrivePermission(runtime *RuntimeContext, token, resourceType string) map[string]interface{} {
	userOpenID := strings.TrimSpace(runtime.UserOpenId())
	if userOpenID == "" {
		result := buildPermissionGrantResult(
			PermissionGrantSkipped,
			"",
			fmt.Sprintf("Resource was created with bot identity, but no current CLI user open_id is configured, so current user %s was not granted. You can retry later or continue using bot identity.", permissionGrantPermMessage()),
			"No current user identity (not logged in or session expired).",
		)
		fmt.Fprintf(runtime.IO().ErrOut, "Warning: resource was created with bot identity, but no current user open_id is configured, so auto-grant was skipped. Run `lark-cli auth login` and retry, or grant permission manually.\n")
		return result
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

	_, err := runtime.CallAPITyped(
		"POST",
		fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members", validate.EncodePathSegment(token)),
		map[string]interface{}{
			"type":              resourceType,
			"need_notification": false,
		},
		body,
	)
	if err != nil {
		errMsg := compactPermissionGrantError(err)
		result := buildPermissionGrantResult(
			PermissionGrantFailed,
			userOpenID,
			fmt.Sprintf("Resource was created, but granting current user %s failed: %s. You can retry later or continue using bot identity.", permissionGrantPermMessage(), errMsg),
			fmt.Sprintf("Auto-grant failed: %s. The app may lack the required scope or the resource restricts permission changes.", errMsg),
		)
		// Best-effort: when the underlying error is permission-class
		// (lark code 99991672/99991679), surface lark_code, required_scope
		// and console_url so agents can guide users straight to the dev
		// console. Overrides the generic hint with a more actionable one
		// when console_url is available.
		annotateGrantPermissionError(runtime, result, err)
		fmt.Fprintf(runtime.IO().ErrOut, "Warning: resource was created, but auto-grant failed: %s. Retry later or grant permission manually.\n", errMsg)
		return result
	}

	return buildPermissionGrantResult(
		PermissionGrantGranted,
		userOpenID,
		fmt.Sprintf("Granted the current CLI user %s on the new %s.", permissionGrantPermMessage(), permissionTargetLabel(resourceType)),
		"",
	)
}

func buildPermissionGrantResult(status, userOpenID, message, reason string) map[string]interface{} {
	result := map[string]interface{}{
		"status":  status,
		"perm":    permissionGrantPerm,
		"message": message,
	}
	if userOpenID != "" {
		result["user_open_id"] = userOpenID
		result["member_type"] = "openid"
	}
	if status == PermissionGrantSkipped {
		result["hint"] = reason + " Run `lark-cli auth login` and retry, or grant permission manually via the Lark document UI."
	} else if status == PermissionGrantFailed {
		result["hint"] = reason + " Retry later or grant permission manually via the Lark document UI."
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

// annotateGrantPermissionError enriches a failed permission_grant result with
// structured fields (lark_code / required_scope / console_url) when the
// underlying error is a typed *errs.PermissionError. The typed error produced
// by errclass.BuildAPIError already carries MissingScopes + ConsoleURL for
// top-level failures; this helper covers best-effort sub-calls whose error is
// folded into a result map instead of propagated.
//
// When console_url is available, the existing generic hint is overridden with
// a more actionable one pointing at the developer console — that's the
// concrete next step a user can take.
func annotateGrantPermissionError(runtime *RuntimeContext, result map[string]interface{}, err error) {
	if runtime == nil || result == nil || err == nil {
		return
	}
	code, scopes, ok := permissionGrantErrorFacts(err)
	if !ok {
		return
	}
	if code != 0 {
		result["lark_code"] = code
	}

	if len(scopes) == 0 {
		return
	}
	recommended := registry.SelectRecommendedScopeFromStrings(scopes, "tenant")
	if recommended == "" {
		return
	}
	result["required_scope"] = recommended

	if runtime.Config == nil || runtime.Config.AppID == "" {
		return
	}
	consoleURL := registry.BuildConsoleScopeURL(runtime.Config.Brand, runtime.Config.AppID, recommended)
	if consoleURL == "" {
		return
	}
	result["console_url"] = consoleURL
	// Override the generic hint: pointing at the dev console is more actionable
	// than the generic "retry later" fallback set by buildPermissionGrantResult.
	result["hint"] = fmt.Sprintf(
		"App is missing the %q scope; enable it in the developer console (see console_url), then retry.",
		recommended,
	)
}

// permissionGrantErrorFacts extracts the Lark code and missing scopes from a
// permission-class error. A typed *errs.PermissionError carries both directly.
// Non-permission errors report ok=false.
func permissionGrantErrorFacts(err error) (code int, scopes []string, ok bool) {
	var permErr *errs.PermissionError
	if errors.As(err, &permErr) {
		return permErr.Code, permErr.MissingScopes, true
	}
	return 0, nil, false
}
