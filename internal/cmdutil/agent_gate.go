// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"

	"github.com/larksuite/cli/internal/envvars"
)

// AgentIdentityGateEnabled reports whether the Agent Employee identity gate
// applies to generated API commands. It is on by default: generated commands
// and shortcuts both filter by identity, and an Agent Employee is a distinct
// identity, so a method that does not declare it is not callable.
//
// The verdict is only ever as good as the metadata behind it. A method is
// agent-callable when its accessTokens names the agent token, and a service
// whose metadata names it nowhere refuses everything it offers — which the
// refusal says, pointing at the service's owning team rather than at the
// profile, because no local change can fix it. Turning the gate off restores
// the pre-Agent behaviour for a caller who needs it before the metadata
// catches up.
//
// On or off, this is a local pre-check, not a permission boundary — the open
// platform decides what an Agent Employee may call.
func AgentIdentityGateEnabled() bool {
	return envvars.Bool(envvars.CliAgentIdentityGate, true)
}

// WarnAgentIdentityGateOffOnce prints the gate-off notice once per Factory on
// its error stream. The guard is per instance so every Factory in a test binary
// warns, not only the first.
//
// Nothing is printed when the gate is simply at its default: a warning about a
// state the user never chose is one line of noise on every command, and the
// advice it carries — the open platform enforces identity — is true whether or
// not the local pre-check ran. A value that does not parse as false falls back
// to that default and is treated the same way: the notice says the value
// disabled the gate, so it is printed only when the value did.
func (f *Factory) WarnAgentIdentityGateOffOnce() {
	if f.IOStreams == nil || f.IOStreams.ErrOut == nil {
		return
	}
	raw, set := envvars.Lookup(envvars.CliAgentIdentityGate)
	if !set || envvars.Bool(envvars.CliAgentIdentityGate, true) {
		return
	}
	f.agentIdentityGateOffWarnOnce.Do(func() {
		fmt.Fprintf(f.IOStreams.ErrOut,
			"warning: Agent Employee metaAPI identity gate disabled by %s=%s; "+
				"identity is enforced by the open platform only\n",
			envvars.CliAgentIdentityGate, raw)
	})
}
