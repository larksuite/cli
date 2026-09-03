// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/pflag"
)

// GlobalOptions are the root-level flags shared by bootstrap parsing and the
// actual Cobra command tree. Profile is the parsed --profile value; HideProfile
// is a build-time policy — when true, --profile stays parseable but is marked
// hidden from help and shell completion.
type GlobalOptions struct {
	Profile     string
	HideProfile bool
}

// RegisterGlobalFlags registers the root-level persistent flags on fs and
// applies any visibility policy encoded in opts. Pure function: no disk,
// network, or environment reads — the caller decides HideProfile.
func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
	fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
	if opts.HideProfile {
		_ = fs.MarkHidden("profile")
	}
}

// isSingleAppMode reports whether the on-disk config has at most one app.
// Missing configs are treated as single-app since --profile is meaningless
// until at least two profiles exist. Intended for the Execute entry point —
// buildInternal must not call this directly to stay state-free.
func isSingleAppMode() bool {
	raw, err := core.LoadMultiAppConfig()
	if err != nil || raw == nil {
		return true
	}
	return len(raw.Apps) <= 1
}
