// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

const (
	CliAppID             = "LARKSUITE_CLI_APP_ID"
	CliAppSecret         = "LARKSUITE_CLI_APP_SECRET"
	CliBrand             = "LARKSUITE_CLI_BRAND"
	CliUserAccessToken   = "LARKSUITE_CLI_USER_ACCESS_TOKEN"
	CliTenantAccessToken = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"
	CliDefaultAs         = "LARKSUITE_CLI_DEFAULT_AS"
	CliProfile           = "LARKSUITE_CLI_PROFILE"
	CliStrictMode        = "LARKSUITE_CLI_STRICT_MODE"

	// Sidecar proxy (auth proxy mode)
	CliAuthProxy = "LARKSUITE_CLI_AUTH_PROXY" // sidecar HTTP address, e.g. "http://127.0.0.1:16384"
	CliProxyKey  = "LARKSUITE_CLI_PROXY_KEY"  // HMAC signing key shared with sidecar

	// Content safety scanning mode
	CliContentSafetyMode = "LARKSUITE_CLI_CONTENT_SAFETY_MODE"

	// Escape hatch for the risk-declaration gate: when set to a truthy value,
	// a command whose declared risk is outside the closed taxonomy is treated
	// as the highest tier (confirmation still required) instead of being
	// refused outright. It can only soften "refuse" into "confirm" — there is
	// no value that lets an unrecognised risk run unconfirmed.
	CliAllowInvalidRisk = "LARKSUITE_CLI_ALLOW_INVALID_RISK"

	CliAgentName  = "LARKSUITE_CLI_AGENT_NAME"
	CliAgentTrace = "LARKSUITE_CLI_AGENT_TRACE"

	CliProxyEnable  = "LARKSUITE_CLI_PROXY_ENABLE"
	CliProxyAddress = "LARKSUITE_CLI_PROXY_ADDRESS"
	CliCAPath       = "LARKSUITE_CLI_CA_PATH"
)
