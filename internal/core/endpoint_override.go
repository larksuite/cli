// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"os"
	"strings"
)

// EndpointDomainEnv is the environment variable that, when set to a bare
// domain (e.g. "feishu-boe.cn"), overrides all resolved endpoints. It takes
// precedence over brand-based resolution so a single variable can retarget the
// whole CLI at a non-production environment (boe / pre).
const EndpointDomainEnv = "LARKSUITE_CLI_ENDPOINT_DOMAIN"

// endpointOverride returns endpoints derived from EndpointDomainEnv.
// The second return value is false when the variable is unset or blank,
// in which case callers fall back to brand-based resolution.
func endpointOverride() (Endpoints, bool) {
	domain := strings.TrimSpace(os.Getenv(EndpointDomainEnv))
	if domain == "" {
		return Endpoints{}, false
	}
	return Endpoints{
		Open:     "https://open." + domain,
		Accounts: "https://accounts." + domain,
		MCP:      "https://mcp." + domain,
		AppLink:  "https://applink." + domain,
	}, true
}
