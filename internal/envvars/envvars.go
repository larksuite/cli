// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package envvars

import "os"

const (
	CliAppID             = "LARKSUITE_CLI_APP_ID"
	CliAppSecret         = "LARKSUITE_CLI_APP_SECRET"
	CliBrand             = "LARKSUITE_CLI_BRAND"
	CliUserAccessToken   = "LARKSUITE_CLI_USER_ACCESS_TOKEN"
	CliTenantAccessToken = "LARKSUITE_CLI_TENANT_ACCESS_TOKEN"
	CliDefaultAs         = "LARKSUITE_CLI_DEFAULT_AS"
	CliStrictMode        = "LARKSUITE_CLI_STRICT_MODE"

	// Sidecar proxy (auth proxy mode)
	CliAuthProxy = "LARKSUITE_CLI_AUTH_PROXY" // sidecar HTTP address, e.g. "http://127.0.0.1:16384"
	CliProxyKey  = "LARKSUITE_CLI_PROXY_KEY"  // HMAC signing key shared with sidecar

	// Content safety scanning mode
	CliContentSafetyMode = "LARKSUITE_CLI_CONTENT_SAFETY_MODE"

	CliAgentName  = "LARKSUITE_CLI_AGENT_NAME"
	CliAgentTrace = "LARKSUITE_CLI_AGENT_TRACE"

	CliProxyEnable  = "LARKSUITE_CLI_PROXY_ENABLE"
	CliProxyAddress = "LARKSUITE_CLI_PROXY_ADDRESS"
	CliCAPath       = "LARKSUITE_CLI_CA_PATH"
)

// HasEnvCredentials reports whether any of the environment variables that feed
// the env credential provider (app id/secret or an access token) are set. It
// lets callers such as `doctor` distinguish "nothing configured at all" from
// "credentials supplied via environment but no config.json on disk", so the
// user isn't told to run `config init` when env credentials are the intended
// setup.
func HasEnvCredentials() bool {
	for _, key := range []string{CliAppID, CliAppSecret, CliUserAccessToken, CliTenantAccessToken} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}
