// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// AddAPIIdentityFlag registers the standard --as flag shape used by api/service commands.
// supported lists the identities the command permits, rendered in both help and
// shell completion. A nil slice keeps the unrestricted default shape ("identity
// type: user | bot"); a non-nil slice (including an empty one) is rendered
// verbatim so restricted methods stop advertising identities that CheckIdentity
// will reject (see larksuite/cli#2206).
func AddAPIIdentityFlag(ctx context.Context, cmd *cobra.Command, f *Factory, target *string, supported []string) {
	if supported == nil {
		supported = []string{"user", "bot"}
	}
	addIdentityFlag(ctx, cmd, f, target, identityFlagConfig{
		defaultValue:     "",
		usage:            "identity type: " + strings.Join(supported, " | "),
		completionValues: supported,
	})
}

// AddShortcutIdentityFlag registers the standard --as flag shape used by shortcuts.
func AddShortcutIdentityFlag(ctx context.Context, cmd *cobra.Command, f *Factory, authTypes []string) {
	if len(authTypes) == 0 {
		authTypes = []string{"user"}
	}
	addIdentityFlag(ctx, cmd, f, nil, identityFlagConfig{
		defaultValue:     "",
		usage:            "identity type: " + strings.Join(authTypes, " | "),
		completionValues: authTypes,
	})
}

type identityFlagConfig struct {
	defaultValue     string
	usage            string
	completionValues []string
}

// addIdentityFlag centralizes --as registration and strict-mode UX.
// When strict mode is active, the flag is still accepted for compatibility
// but hidden from help/completion and locked to the forced identity by default.
func addIdentityFlag(ctx context.Context, cmd *cobra.Command, f *Factory, target *string, cfg identityFlagConfig) {
	if forced := f.ResolveStrictMode(ctx).ForcedIdentity(); forced != "" {
		// Keep registering --as in strict mode even though it is hidden.
		// This preserves parser compatibility for existing invocations that still pass
		// --as, and keeps downstream GetString("as") / ResolveAs paths stable.
		// The usage text below is effectively placeholder text because the flag is hidden.
		registerIdentityFlag(cmd, target, string(forced),
			fmt.Sprintf("identity locked to %s by strict mode (admin-managed)", forced))
		_ = cmd.Flags().MarkHidden("as")
		return
	}

	registerIdentityFlag(cmd, target, cfg.defaultValue, cfg.usage)
	RegisterFlagCompletion(cmd, "as", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return cfg.completionValues, cobra.ShellCompDirectiveNoFileComp
	})
}

func registerIdentityFlag(cmd *cobra.Command, target *string, defaultValue, usage string) {
	if target != nil {
		cmd.Flags().StringVar(target, "as", defaultValue, usage)
		return
	}
	cmd.Flags().String("as", defaultValue, usage)
}
