// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"maps"
	"slices"
	"strings"
)

// Canonicalize returns the normalized copy of a declaration: the defaults
// that used to be applied at registration time, made explicit. Projections
// and the compatibility view are both built from the canonical form, so
// round-trip checks compare against Canonicalize(input), never the raw input.
func Canonicalize(def KeyDefinition) KeyDefinition {
	out := deepCopyDefinition(def)
	if out.SubscriptionType == "" {
		out.SubscriptionType = SubTypeEvent
	}
	if out.BufferSize > MaxBufferSize {
		out.BufferSize = MaxBufferSize
	}
	if out.BufferSize <= 0 {
		out.BufferSize = DefaultBufferSize
	}
	if out.Workers <= 0 {
		out.Workers = 1
	}
	return out
}

// DerivedDomain returns the declaration's explicit Domain, or the key's first
// dot segment. The derived value feeds the Descriptor only — it is never
// written back into the compatibility view, so a declaration that said
// nothing keeps saying nothing in legacy JSON output.
func DerivedDomain(def *KeyDefinition) string {
	if def.Domain != "" {
		return def.Domain
	}
	domain, _, found := strings.Cut(def.Key, ".")
	if !found || domain == "" {
		return def.Key
	}
	return domain
}

// deepCopyDefinition clones every mutable member so neither the caller's
// declaration nor a compiled entry can be changed through the other.
func deepCopyDefinition(def KeyDefinition) KeyDefinition {
	out := def
	out.Params = slices.Clone(def.Params)
	for i := range out.Params {
		out.Params[i].Values = slices.Clone(def.Params[i].Values)
	}
	out.Scopes = slices.Clone(def.Scopes)
	out.AuthTypes = slices.Clone(def.AuthTypes)
	out.RequiredConsoleEvents = slices.Clone(def.RequiredConsoleEvents)
	if def.Schema.Native != nil {
		spec := *def.Schema.Native
		spec.Raw = slices.Clone(def.Schema.Native.Raw)
		out.Schema.Native = &spec
	}
	if def.Schema.Custom != nil {
		spec := *def.Schema.Custom
		spec.Raw = slices.Clone(def.Schema.Custom.Raw)
		out.Schema.Custom = &spec
	}
	out.Schema.FieldOverrides = maps.Clone(def.Schema.FieldOverrides)
	return out
}
