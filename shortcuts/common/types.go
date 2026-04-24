// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	"github.com/spf13/cobra"
)

// Flag.Input source constants.
const (
	File  = "file"  // support @path to read value from a file
	Stdin = "stdin" // support - to read value from stdin
)

// Flag describes a CLI flag for a shortcut.
type Flag struct {
	Name     string // flag name (e.g. "calendar-id")
	Type     string // "string" (default) | "bool" | "int" | "string_array"
	Default  string // default value as string
	Desc     string // help text
	Hidden   bool   // hidden from --help, still readable at runtime
	Required bool
	Enum     []string // allowed values (e.g. ["asc", "desc"]); empty means no constraint
	Input    []string // extra input sources: File (@path), Stdin (-); empty = flag value only
}

// Shortcut represents a high-level CLI command.
type Shortcut struct {
	Service     string
	Command     string
	Description string
	Risk        string   // "read" | "write" | "high-risk-write" (empty defaults to "read")
	Scopes      []string // default scopes (fallback when UserScopes/BotScopes are empty)
	UserScopes  []string // optional: user-identity scopes (overrides Scopes when non-empty)
	BotScopes   []string // optional: bot-identity scopes (overrides Scopes when non-empty)

	// Declarative fields (new framework).
	AuthTypes []string // supported identities: "user", "bot" (default: ["user"])
	Flags     []Flag   // flag definitions; --dry-run is auto-injected
	HasFormat bool     // auto-inject --format flag (json|pretty|table|ndjson|csv)
	Tips      []string // optional tips shown in --help output
	Hidden    bool     // hide from --help / tab completion (still executable); use when deprecating a command in favor of a replacement

	// Business logic hooks.
	DryRun   func(ctx context.Context, runtime *RuntimeContext) *DryRunAPI // optional: framework prints & returns when --dry-run is set
	Validate func(ctx context.Context, runtime *RuntimeContext) error      // optional pre-execution validation
	Execute  func(ctx context.Context, runtime *RuntimeContext) error      // main logic

	// PostMount is an optional hook called after the cobra.Command is fully
	// configured (flags registered, tips set) and after parent.AddCommand(cmd)
	// has attached it to the parent. Use it to install custom help functions or
	// tweak the command; cmd.Parent() is available at this point.
	PostMount func(cmd *cobra.Command)
}

// ScopesForIdentity returns the scopes applicable for the given identity.
// If identity-specific scopes (UserScopes/BotScopes) are set, they take
// precedence over the default Scopes.
func (s *Shortcut) ScopesForIdentity(identity string) []string {
	switch identity {
	case "user":
		if len(s.UserScopes) > 0 {
			return s.UserScopes
		}
	case "bot":
		if len(s.BotScopes) > 0 {
			return s.BotScopes
		}
	}
	return s.Scopes
}
