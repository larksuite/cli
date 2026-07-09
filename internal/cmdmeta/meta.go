// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package cmdmeta is the single source of truth for command metadata that the
// policy engine, the hook selector, and help rendering consume. It wraps the
// existing cmdutil annotations (risk_level, supportedIdentities) and adds the
// "domain" axis that the hook selector and Rule path globs need, plus the
// affordance ref (service, method id) that lets service-method and shortcut
// help share one usage-guidance lookup path.
//
// Three axes:
//
//   - Domain     - business domain ("im", "docs", "contact", ...). Inherited
//     from the nearest ancestor when not set on the command
//     itself. Stored on a new annotation key (the cmdutil
//     risk_level / supportedIdentities keys are left untouched
//     for backward compatibility).
//   - Risk       - "read" | "write" | "high-risk-write". Inherited like
//     Domain. Reuses cmdutil.SetRisk / GetRisk under the hood.
//   - Identities - allowed identity set. Child explicit override semantics:
//     the first ancestor (including self) with a non-nil set
//     wins. Reuses cmdutil.SetSupportedIdentities /
//     GetSupportedIdentities.
//
// Missing values are returned as the zero value with ok=false (where the
// signature exposes it). Interpretation is up to the consumer: the policy
// engine treats a missing risk as fail-closed when a Rule is registered
// without AllowUnannotated=true, and as allow otherwise. Identities still
// defaults to ALLOW. Do not synthesise defaults here -- let each consumer
// decide.
package cmdmeta

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// Source identifies how a command entered the repository-owned command tree.
type Source string

const (
	SourceBuiltin  Source = "builtin"
	SourceShortcut Source = "shortcut"
	SourceService  Source = "service"
)

const (
	// domainAnnotationKey is the cobra Annotation key for the business domain.
	// Kept distinct from cmdutil.* keys so this package can evolve without
	// disturbing existing readers.
	domainAnnotationKey = "cmdmeta.domain"

	sourceAnnotationKey    = "cmdmeta.source"
	generatedAnnotationKey = "cmdmeta.generated"

	// affordance{Service,Method}Key locate the command's usage-guidance overlay
	// entry (see internal/affordance). Both service-method commands and
	// +-prefixed shortcuts set these so help rendering shares one lookup path.
	affordanceServiceKey = "cmdmeta.affordance.service"
	affordanceMethodKey  = "cmdmeta.affordance.method"
)

// Meta groups the three command-level metadata axes consumed by the policy
// engine and hook selectors.
type Meta struct {
	Domain     string
	Risk       string
	Identities []string
}

// Apply writes metadata onto a cobra command. Empty fields are skipped: pass
// the value via the underlying cmdutil setter if you need to write an empty
// string / empty slice explicitly.
func Apply(cmd *cobra.Command, m Meta) {
	if m.Domain != "" {
		SetDomain(cmd, m.Domain)
	}
	if m.Risk != "" {
		cmdutil.SetRisk(cmd, m.Risk)
	}
	if m.Identities != nil {
		cmdutil.SetSupportedIdentities(cmd, m.Identities)
	}
}

// Get resolves the effective metadata for a command, walking up the parent
// chain for Domain, Risk, and Identities. All three axes use the same
// nearest-ancestor-wins rule.
//
// Identities note: cmdutil.GetSupportedIdentities collapses both the
// "annotation absent" and "annotation set to empty string" cases to nil.
// A child cannot therefore express "deny inheritance" with an empty
// annotation; the walk simply continues up the parent chain when nil is
// returned. To override a parent, the child must set a non-empty slice
// (e.g. ["bot"]).
func Get(cmd *cobra.Command) Meta {
	risk, _ := Risk(cmd)
	return Meta{
		Domain:     Domain(cmd),
		Risk:       risk,
		Identities: Identities(cmd),
	}
}

// SetDomain stores the domain annotation on a single command (no
// inheritance is performed on write).
func SetDomain(cmd *cobra.Command, domain string) {
	if domain == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[domainAnnotationKey] = domain
}

// SetSource stores the command source on a single command. The generated flag
// is written explicitly so child commands can opt out of inherited service
// metadata.
func SetSource(cmd *cobra.Command, source Source, generated bool) {
	if source == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[sourceAnnotationKey] = string(source)
	if generated {
		cmd.Annotations[generatedAnnotationKey] = "true"
	} else {
		cmd.Annotations[generatedAnnotationKey] = "false"
	}
}

// SetAffordanceRef records which affordance overlay entry (service, method id)
// a command maps to, so help rendering can look up its usage guidance. Stored
// on the command itself (no inheritance): each method / shortcut owns its ref.
// A no-op if either coordinate is empty.
func SetAffordanceRef(cmd *cobra.Command, service, method string) {
	if service == "" || method == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[affordanceServiceKey] = service
	cmd.Annotations[affordanceMethodKey] = method
}

// AffordanceRef returns the command's own affordance overlay coordinates.
// ok is false when the command carries no ref.
func AffordanceRef(cmd *cobra.Command) (service, method string, ok bool) {
	if cmd.Annotations == nil {
		return "", "", false
	}
	service = cmd.Annotations[affordanceServiceKey]
	method = cmd.Annotations[affordanceMethodKey]
	if service == "" || method == "" {
		return "", "", false
	}
	return service, method, true
}

// Domain returns the nearest-ancestor domain for the command. Empty string
// when no ancestor has the annotation -- this is the "unknown" state the
// policy engine must treat as ALLOW.
func Domain(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations == nil {
			continue
		}
		if v, ok := c.Annotations[domainAnnotationKey]; ok && v != "" {
			return v
		}
	}
	return ""
}

// SourceOf returns the nearest-ancestor command source.
func SourceOf(cmd *cobra.Command) (Source, bool) {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations == nil {
			continue
		}
		if v := c.Annotations[sourceAnnotationKey]; v != "" {
			return Source(v), true
		}
	}
	return "", false
}

// Generated returns the nearest generated annotation. An explicit false on a
// child command stops inheritance from a generated parent.
func Generated(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Annotations == nil {
			continue
		}
		if v, ok := c.Annotations[generatedAnnotationKey]; ok {
			return v == "true"
		}
	}
	return false
}

// Risk returns the nearest-ancestor risk level (via cmdutil.GetRisk).
// ok=false signals "unknown" -- the policy engine treats this as
// fail-closed (deny with risk_not_annotated) whenever a Rule without
// AllowUnannotated=true is active, and as allow otherwise.
func Risk(cmd *cobra.Command) (level string, ok bool) {
	for c := cmd; c != nil; c = c.Parent() {
		if level, ok = cmdutil.GetRisk(c); ok {
			return level, true
		}
	}
	return "", false
}

// Identities returns the first non-nil identity set found while walking up
// the parent chain. nil signals "unknown" -- the policy engine treats this
// as ALLOW.
//
// cmdutil.GetSupportedIdentities returns nil when the annotation is absent
// or empty; an explicit non-empty set (even ["user"] alone) stops the walk.
func Identities(cmd *cobra.Command) []string {
	for c := cmd; c != nil; c = c.Parent() {
		if ids := cmdutil.GetSupportedIdentities(c); ids != nil {
			return ids
		}
	}
	return nil
}
