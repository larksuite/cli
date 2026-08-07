// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"github.com/larksuite/cli/brand"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/surface"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// globalFlagTargets maps each root persistent flag to the command capability
// it belongs to. A new domain-tied global flag must add a row.
var globalFlagTargets = map[string]surface.CommandID{
	"profile": surface.CommandProfile,
}

// flagGateAnnotation distinguishes a surface-retired flag from one hidden
// cosmetically (single-app mode force-shows the latter in root help).
const flagGateAnnotation = "lark:surface_concealed_flag"

// applyPluginFlagGate hides and rejects global flags whose exact command
// capability is absent from this build. It is called only by the explicit
// distribution presentation pass.
func applyPluginFlagGate(root *cobra.Command, plan *surface.Plan) {
	for flagName, target := range globalFlagTargets {
		if plan.CanReference(target) {
			continue
		}
		fl := root.PersistentFlags().Lookup(flagName)
		if fl == nil {
			continue
		}
		fl.Hidden = true
		if fl.Annotations == nil {
			fl.Annotations = map[string][]string{}
		}
		fl.Annotations[flagGateAnnotation] = []string{"true"}
		fl.Value = &gatedFlagValue{name: flagName, inner: fl.Value}
	}
}

// installEnvironmentProfileGate applies the profile capability decision to
// the environment equivalent of --profile. A gated pflag Value can reject an
// argv token, but an environment selector has no token for Cobra to parse, so
// it needs an invocation guard of its own. The return value reports whether a
// guard was installed so Build can stop before plugin Startup.
func installEnvironmentProfileGate(
	root *cobra.Command,
	inv cmdutil.InvocationContext,
	plan *surface.Plan,
) bool {
	if inv.ProfileSource != brand.ProfileFromEnvironment ||
		plan.CanReference(surface.CommandProfile) {
		return false
	}

	makeErr := func() error {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"environment variable %q is not supported by this build",
			envvars.CliProfile,
		).
			WithParam(envvars.CliProfile).
			WithHint("remove %s from the process environment and retry", envvars.CliProfile)
	}
	installFatalGuard(root, makeErr)
	return true
}

func isPolicyGatedFlag(fl *pflag.Flag) bool {
	return fl != nil && fl.Annotations[flagGateAnnotation] != nil
}

// gatedFlagValue rejects at parse time, before cobra's help/version fast
// paths (which never reach PersistentPreRunE). Its Set error carries
// cobra's own unknown-flag wording so the root FlagErrorFunc classifies it
// as an ordinary unknown flag without exposing policy state. Cobra may add
// different parse context on root/group paths than on leaf commands.
type gatedFlagValue struct {
	name  string
	inner pflag.Value
}

func (g *gatedFlagValue) String() string { return g.inner.String() }
func (g *gatedFlagValue) Type() string   { return g.inner.Type() }
func (g *gatedFlagValue) Set(string) error {
	// Intermediate parse error, not a final envelope: pflag wraps it and
	// the root FlagErrorFunc (flagDidYouMean) converts it to the typed
	// unknown-flag validation error.
	return errors.New("unknown flag: --" + g.name) //nolint:forbidigo // intermediate parse error; flagDidYouMean emits the typed envelope
}
