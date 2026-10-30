// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"net/url"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/urlrewrite"
)

// NoRefreshTokenCause states why a login can end up without a refresh_token.
// Requesting offline_access is not enough: the token endpoint only issues a
// refresh_token after the app turns on "Refresh user_access_token" in its
// security settings and publishes an app version containing that change.
// Until then it still echoes offline_access in the granted scope.
const NoRefreshTokenCause = `the app has not enabled "Refresh user_access_token" in its developer-console security settings, or has not published an app version since enabling it (requesting offline_access alone does not issue a refresh_token)`

// NoRefreshTokenSettingsURL returns the developer-console security settings
// page where "Refresh user_access_token" is turned on for appID.
func NoRefreshTokenSettingsURL(brand core.LarkBrand, appID string) string {
	if appID == "" {
		return ""
	}
	return urlrewrite.Rewrite(fmt.Sprintf("%s/app/%s/safe", core.ResolveOpenBaseURL(brand), url.PathEscape(appID)))
}

// NoRefreshTokenFix tells the user how to make future logins renewable.
func NoRefreshTokenFix(brand core.LarkBrand, appID string) string {
	fix := `enable "Refresh user_access_token" in the app's security settings, create and publish a new app version, then log in again`
	if settingsURL := NoRefreshTokenSettingsURL(brand, appID); settingsURL != "" {
		fix += " (" + settingsURL + ")"
	}
	return fix
}
