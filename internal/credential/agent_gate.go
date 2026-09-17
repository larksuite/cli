// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"fmt"

	"github.com/larksuite/cli/internal/envvars"
)

// agentScopePrecheckEnabled reports whether the local scope pre-check runs for
// an Agent Employee; off by default. Off is a fuse, not a permission bypass:
// the open platform still enforces scopes.
func agentScopePrecheckEnabled() bool {
	return envvars.Bool(envvars.CliAgentScopePrecheck, false)
}

// WarnAgentScopePrecheckOffOnce prints, once per provider, that the scope
// pre-check is disabled. It lives here for the same reason as
// WarnAgentScopeSnapshotOnce.
//
// Nothing is printed when the pre-check is simply at its default: an Agent
// Employee runs every scope-declaring command through here, so warning about
// an unchosen default would put a line on stderr for each of them. A value
// that does not parse as false leaves the default in place and is treated the
// same way: the notice blames the value for the switch-off, so it is printed
// only when the value actually spelled false.
func (p *DefaultTokenProvider) WarnAgentScopePrecheckOffOnce() {
	if p.errOut == nil {
		return
	}
	raw, set := envvars.Lookup(envvars.CliAgentScopePrecheck)
	if !set || envvars.Bool(envvars.CliAgentScopePrecheck, true) {
		return
	}
	p.scopePrecheckOffWarnOnce.Do(func() {
		fmt.Fprintf(p.errOut,
			"warning: Agent Employee scope pre-check disabled by %s=%s; "+
				"permissions are enforced by the open platform only\n",
			envvars.CliAgentScopePrecheck, raw)
	})
}
