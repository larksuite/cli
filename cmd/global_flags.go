// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import "github.com/spf13/pflag"

// GlobalOptions are the root-level flags shared by bootstrap parsing and the
// actual Cobra command tree.
type GlobalOptions struct {
	Profile string
}

// RegisterGlobalFlags registers the root-level persistent flags.
func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
	fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
}
