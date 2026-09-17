// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/agentsupport"
)

const skipAuthCheckKey = "skipAuthCheck"
const annotationSupportedIdentities = "lark:supportedIdentities"
const annotationAgentSupport = "lark:agentSupport"
const annotationAgentServiceAnnotated = "lark:agentServiceAnnotated"

// SetSupportedIdentities marks which identities a command supports.
func SetSupportedIdentities(cmd *cobra.Command, identities []string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationSupportedIdentities] = strings.Join(identities, ",")
}

// GetSupportedIdentities returns the declared identities, or nil if not declared.
func GetSupportedIdentities(cmd *cobra.Command) []string {
	v, ok := cmd.Annotations[annotationSupportedIdentities]
	if !ok || v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// SetAgentSupport records the shipped verdict on whether an Agent Employee may
// run this command. For a shortcut it comes from shortcuts/agent_support.json,
// a hand-maintained table; for a generated API method it comes from the
// Catalog's accessTokens. Either way it is the only thing that decides an AAT
// profile's access -- see Factory.CheckShortcutPrincipal. An empty verdict
// writes nothing, so an unrecorded command reads as unknown and stays
// available.
func SetAgentSupport(cmd *cobra.Command, support agentsupport.Verdict) {
	if support == agentsupport.Unknown {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationAgentSupport] = string(support)
}

// GetAgentSupport returns the recorded verdict, or "" when none was recorded.
func GetAgentSupport(cmd *cobra.Command) agentsupport.Verdict {
	return agentsupport.Verdict(cmd.Annotations[annotationAgentSupport])
}

// SetAgentServiceAnnotated records that the owning service has at least one
// agent-marked method. It only selects the refusal wording: a denied command
// in an otherwise agent-marked service can plausibly be unlocked by a
// capability grant, while one in a service with no marker at all cannot be
// unlocked locally at all. Generated API commands carry it from their
// service's metadata; shortcuts are stamped with it at mount so they never
// take the wording that blames the service's metadata.
func SetAgentServiceAnnotated(cmd *cobra.Command, annotated bool) {
	if !annotated {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationAgentServiceAnnotated] = "true"
}

// AgentServiceAnnotated reports whether the owning service has any agent-marked
// method. Absent reads as false, which selects the wording that tells the
// reader no local grant will help -- the safe answer when nothing claimed
// otherwise.
func AgentServiceAnnotated(cmd *cobra.Command) bool {
	return cmd.Annotations[annotationAgentServiceAnnotated] == "true"
}

// DisableAuthCheck marks a command (and all its children) as not requiring auth.
func DisableAuthCheck(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipAuthCheckKey] = "true"
}

// IsAuthCheckDisabled returns true if the command or any ancestor has auth check disabled.
func IsAuthCheckDisabled(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations != nil && c.Annotations[skipAuthCheckKey] == "true" {
			return true
		}
	}
	return false
}
