// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

const (
	// CliAppID is the app ID environment variable consumed by the CLI.
	CliAppID = "LARKSUITE_CLI_APP_ID"

	// CliAppSecret is the app secret environment variable consumed by the CLI.
	CliAppSecret = "LARKSUITE_CLI_APP_SECRET"

	// CliBrand selects the tenant brand environment variable consumed by the CLI.
	CliBrand = "LARKSUITE_CLI_BRAND"

	// CliUserAccessToken is the user access token override environment variable.
	CliUserAccessToken = "LARKSUITE_CLI_USER_ACCESS_TOKEN"

	// CliTenantAccessToken is the tenant access token override environment variable.
	CliTenantAccessToken = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"

	// CliDefaultAs selects the default identity environment variable.
	CliDefaultAs = "LARKSUITE_CLI_DEFAULT_AS"

	// CliStrictMode selects the strict identity mode environment variable.
	CliStrictMode = "LARKSUITE_CLI_STRICT_MODE"

	// CliAuthProxy is the auth sidecar HTTP address environment variable.
	CliAuthProxy = "LARKSUITE_CLI_AUTH_PROXY" // sidecar HTTP address, e.g. "http://127.0.0.1:16384"

	// CliProxyKey is the shared HMAC signing key environment variable for the sidecar.
	CliProxyKey = "LARKSUITE_CLI_PROXY_KEY" // HMAC signing key shared with sidecar

	// CliSecEnable enables sec plugin mode from the environment.
	CliSecEnable = "LARKSUITE_CLI_SEC_ENABLE"

	// CliSecProxy sets the fixed sec plugin HTTP proxy address.
	CliSecProxy = "LARKSUITE_CLI_SEC_PROXY"

	// CliSecCA points to an extra PEM bundle trusted by sec plugin mode.
	CliSecCA = "LARKSUITE_CLI_SEC_CA"

	// CliSecAuth enables placeholder-token auth mode for sec plugin flows.
	CliSecAuth = "LARKSUITE_CLI_SEC_AUTH"

	// CliContentSafetyMode selects the content safety scanning mode.
	CliContentSafetyMode = "LARKSUITE_CLI_CONTENT_SAFETY_MODE"

	CliAgentTrace = "LARKSUITE_CLI_AGENT_TRACE"
)
