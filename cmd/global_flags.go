// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/spf13/pflag"
)

// GlobalOptions are the root-level flags shared by bootstrap parsing and the
// Cobra command tree. HideProfile, when true, keeps --profile parseable but
// hidden from help and completion. User is raw — env precedence is resolved
// in bootstrap.go.
type GlobalOptions struct {
	Profile     string
	User        string
	HideProfile bool
}

// RegisterGlobalFlags registers the root-level persistent flags on fs.
// Pure: no disk, network, or env reads.
//
// --user is always visible even when --profile is hidden: an asymmetric
// "hide one, show the other" UX would surprise operators expecting them
// to be symmetric companions.
func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
	fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
	fs.StringVar(&opts.User, "user", "", "select a specific user (open_id or username) within the active profile; overrides "+envvars.CliOpenID)
	if opts.HideProfile {
		_ = fs.MarkHidden("profile")
	}
}

// isSingleAppMode reports whether the on-disk config has at most one app.
// Missing configs count as single-app. Called from Execute only;
// buildInternal stays state-free.
func isSingleAppMode() bool {
	raw, err := core.LoadMultiAppConfig()
	if err != nil || raw == nil {
		return true
	}
	return len(raw.Apps) <= 1
}
