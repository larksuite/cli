// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

// Common authentication paths used for logging and API calls.
const (
	// PathDeviceAuthorization is the endpoint for device authorization.
	PathDeviceAuthorization = "/oauth/v1/device_authorization"
	// PathAppRegistration is the endpoint for application registration.
	PathAppRegistration = "/oauth/v1/app/registration"
	// PathOAuthTokenV2 is the endpoint for requesting an OAuth token (v2).
	PathOAuthTokenV2 = "/open-apis/authen/v2/oauth/token"
	// PathUserInfoV1 is the endpoint for fetching user information.
	PathUserInfoV1 = "/open-apis/authen/v1/user_info"
	// PathApplicationInfoV6Prefix is the prefix endpoint for fetching application info.
	PathApplicationInfoV6Prefix = "/open-apis/application/v6/applications/"
)

// ApplicationInfoPath returns the full API path for querying an application's information.
func ApplicationInfoPath(appId string) string {
	return PathApplicationInfoV6Prefix + appId
}
