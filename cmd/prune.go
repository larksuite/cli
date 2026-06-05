// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// pruneForStrictMode removes commands incompatible with the active strict mode.
func pruneForStrictMode(root *cobra.Command, mode core.StrictMode) {
	pruneIncompatible(root, mode)
	pruneEmpty(root)
}

// pruneIncompatible recursively replaces commands whose annotation declares
// identities incompatible with the forced identity. Commands without annotation are kept.
// Hidden stubs preserve direct execution so users get a strict-mode error instead
// of Cobra's generic "unknown flag" fallback from the parent command.
func pruneIncompatible(parent *cobra.Command, mode core.StrictMode) {
	forced := string(mode.ForcedIdentity())
	var toRemove []*cobra.Command
	var toAdd []*cobra.Command
	for _, child := range parent.Commands() {
		ids := cmdutil.GetSupportedIdentities(child)
		if ids != nil && !slices.Contains(ids, forced) {
			toRemove = append(toRemove, child)
			toAdd = append(toAdd, strictModeStubFrom(child, mode))
			continue
		}
		pruneIncompatible(child, mode)
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
		parent.AddCommand(toAdd...)
	}
}

func strictModeStubFrom(child *cobra.Command, mode core.StrictMode) *cobra.Command {
	// The denial annotations let the hook layer's populateInvocationDenial
	// recognise this command as denied, so the Wrap chain is physically
	// isolated (wrapRunE takes the DeniedByPolicy branch and calls the
	// stub RunE directly). Without these, a plugin Wrapper registered
	// against platform.All() could intercept and silently swallow the
	// strict-mode error -- breaking strict-mode's "hard boundary" contract.
	//
	// Args + PersistentPreRunE overrides mirror cmdpolicy/apply.go::installDenyStub:
	//
	//   - Args=ArbitraryArgs: with DisableFlagParsing the user's flags
	//     look like positional args; the original child's Args validator
	//     (e.g. cobra.NoArgs) would fire BEFORE RunE and produce a
	//     cobra usage error instead of our strict_mode envelope.
	//
	//   - PersistentPreRunE no-op: cmd/auth/auth.go declares a parent
	//     PersistentPreRunE that returns external_provider when env
	//     credentials are set. Cobra's "first wins walking up" would
	//     pick auth's instead of our denial. A leaf-level no-op makes
	//     cobra stop here and proceed to the wrapped RunE.
	//
	// strict-mode keeps its short Message + independent Hint and
	// composes the shared detail.* / wrapped-CommandDeniedError shape
	// by hand; BuildDenialError would override Message with the
	// CommandDeniedError.Error() long form.
	stubMessage := fmt.Sprintf(
		"strict mode is %q, only %s-identity commands are available",
		mode, mode.ForcedIdentity())
	const stubHint = "if the user explicitly wants to switch policy, see `lark-cli config strict-mode --help` (confirm with the user before switching; switching does NOT require re-bind)"
	denial := cmdpolicy.Denial{
		Layer:        cmdpolicy.LayerStrictMode,
		PolicySource: "strict-mode",
		ReasonCode:   "identity_not_supported",
		Reason:       stubMessage,
	}
	// Preserve the original command's annotations (risk_level,
	// lark:supportedIdentities, cmdmeta.domain, ...) and help text so
	// audit / compliance observers can still see what was denied.
	// Stamp the denial annotations on top.
	annotations := make(map[string]string, len(child.Annotations)+2)
	for k, v := range child.Annotations {
		annotations[k] = v
	}
	annotations[cmdpolicy.AnnotationDenialLayer] = cmdpolicy.LayerStrictMode
	annotations[cmdpolicy.AnnotationDenialSource] = "strict-mode"

	return &cobra.Command{
		Use:                child.Use,
		Aliases:            append([]string(nil), child.Aliases...),
		Short:              child.Short,
		Long:               child.Long,
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		Annotations:        annotations,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			c.SilenceUsage = true
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error {
			cd := cmdpolicy.CommandDeniedFromDenial(cmdpolicy.CanonicalPath(c), denial)
			// Legacy *output.ExitError producer: this literal predates the
			// typed error contract introduced by errs/. New denial sites MUST
			// NOT construct *output.ExitError directly — they should return a
			// typed *errs.XxxError once the cmdpolicy framework migrates.
			return &output.ExitError{
				Code: output.ExitValidation,
				Detail: &output.ErrDetail{
					Type:    "command_denied",
					Message: stubMessage,
					Hint:    stubHint,
					Detail:  cmdpolicy.DenialDetailMap(cd),
				},
				Err: cd,
			}
		},
	}
}

// pruneEmpty recursively removes group commands (no Run/RunE) that have
// no remaining subcommands after pruning. If only hidden stubs remain, keep
// the group hidden so direct execution still resolves to the stub path.
func pruneEmpty(parent *cobra.Command) {
	var toRemove []*cobra.Command
	for _, child := range parent.Commands() {
		pruneEmpty(child)
		if child.Run != nil || child.RunE != nil {
			continue
		}
		switch {
		case child.HasAvailableSubCommands():
		case len(child.Commands()) > 0:
			child.Hidden = true
		default:
			toRemove = append(toRemove, child)
		}
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
	}
}
