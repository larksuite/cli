// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	"github.com/spf13/cobra"
)

const (
	File  = "file"  // @path
	Stdin = "stdin" // -
)

type Flag struct {
	Name     string
	Type     string // "string" (default) | "bool" | "int" | "string_array"
	Default  string
	Desc     string
	Hidden   bool
	Required bool
	Enum     []string
	Input    []string // File / Stdin
}

type Shortcut struct {
	Service     string
	Command     string
	Description string
	Risk        string   // "read" | "write" | "high-risk-write" (default "read")
	Scopes      []string // fallback when UserScopes/BotScopes are empty
	UserScopes  []string
	BotScopes   []string

	AuthTypes []string // default ["user"]
	Flags     []Flag   // --dry-run is auto-injected
	HasFormat bool     // auto-inject --format flag
	Tips      []string
	Hidden    bool // hide from --help/tab-completion (still executable)

	DryRun   func(ctx context.Context, runtime *RuntimeContext) *DryRunAPI
	Validate func(ctx context.Context, runtime *RuntimeContext) error
	Execute  func(ctx context.Context, runtime *RuntimeContext) error

	// PostMount runs after parent.AddCommand; cmd.Parent() is available.
	PostMount func(cmd *cobra.Command)
}

// ScopesForIdentity: identity-specific scopes override default Scopes when set.
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
