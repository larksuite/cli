// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

package main

import (
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/sidecar"
)

// buildAllowedHosts extracts the set of allowed target hostnames from
// multiple brand endpoints so the sidecar can serve both feishu and lark clients.
func buildAllowedHosts(endpoints ...core.Endpoints) map[string]bool {
	hosts := make(map[string]bool)
	for _, ep := range endpoints {
		for _, u := range []string{ep.Open, ep.Accounts, ep.MCP} {
			if idx := strings.Index(u, "://"); idx >= 0 {
				hosts[u[idx+3:]] = true
			}
		}
	}
	return hosts
}

// buildAllowedIdentities returns the set of identities the sidecar is allowed to serve,
// based on the trusted-side strict mode / SupportedIdentities configuration.
func buildAllowedIdentities(cfg *core.CliConfig) map[string]bool {
	ids := make(map[string]bool)
	switch {
	case cfg.SupportedIdentities == 0: // unknown/unset → allow both
		ids[sidecar.IdentityUser] = true
		ids[sidecar.IdentityBot] = true
	case cfg.SupportedIdentities&1 != 0: // SupportsUser bit
		ids[sidecar.IdentityUser] = true
	}
	if cfg.SupportedIdentities == 0 || cfg.SupportedIdentities&2 != 0 { // SupportsBot bit
		ids[sidecar.IdentityBot] = true
	}
	return ids
}
