// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

const (
	CliAppID                   = "LARKSUITE_CLI_APP_ID"
	CliAppSecret               = "LARKSUITE_CLI_APP_SECRET"
	CliBrand                   = "LARKSUITE_CLI_BRAND"
	CliUserAccessToken         = "LARKSUITE_CLI_USER_ACCESS_TOKEN"
	CliTenantAccessToken       = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"
	CliAgentAccessToken        = "LARKSUITE_CLI_AGENT_ACCESS_TOKEN"
	CliTenantAccessTokenSource = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN_SOURCE"
	CliDefaultAs               = "LARKSUITE_CLI_DEFAULT_AS"
	CliProfile                 = "LARKSUITE_CLI_PROFILE"
	CliStrictMode              = "LARKSUITE_CLI_STRICT_MODE"

	// Sidecar proxy (auth proxy mode)
	CliAuthProxy = "LARKSUITE_CLI_AUTH_PROXY" // sidecar HTTP address, e.g. "http://127.0.0.1:16384"
	CliProxyKey  = "LARKSUITE_CLI_PROXY_KEY"  // HMAC signing key shared with sidecar

	// Content safety scanning mode
	CliContentSafetyMode = "LARKSUITE_CLI_CONTENT_SAFETY_MODE"

	CliAgentName  = "LARKSUITE_CLI_AGENT_NAME"
	CliAgentTrace = "LARKSUITE_CLI_AGENT_TRACE"

	// Agent Employee local pre-checks. Both are fuses over checks the open
	// platform performs anyway, so both accept the usual boolean spellings. The
	// identity gate is on by default; the scope pre-check is off until the
	// scope metadata it reads is populated.
	CliAgentIdentityGate  = "LARKSUITE_CLI_AGENT_IDENTITY_GATE"
	CliAgentScopePrecheck = "LARKSUITE_CLI_AGENT_SCOPE_PRECHECK"

	CliProxyEnable  = "LARKSUITE_CLI_PROXY_ENABLE"
	CliProxyAddress = "LARKSUITE_CLI_PROXY_ADDRESS"
	CliCAPath       = "LARKSUITE_CLI_CA_PATH"
)
