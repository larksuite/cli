// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import "github.com/spf13/pflag"

// GlobalOptions are the root-level flags shared by bootstrap parsing and the
// actual Cobra command tree.
type GlobalOptions struct {
	Profile   string
	Debug     bool   // --debug flag
	DebugFile string // --debug-file <path> flag
}

// RegisterGlobalFlags registers the root-level persistent flags.
func RegisterGlobalFlags(fs *pflag.FlagSet, opts *GlobalOptions) {
	fs.StringVar(&opts.Profile, "profile", "", "use a specific profile")
	fs.BoolVar(&opts.Debug, "debug", false, "enable debug output to stderr")
	fs.StringVar(&opts.DebugFile, "debug-file", "", "write debug output to file")
}
