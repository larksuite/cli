// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
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

// pruneForAgentPrincipal withholds commands the shipped verdict denies to an
// Agent Employee. It is the principal-kind counterpart to pruneForStrictMode
// and deliberately mirrors its shape, so the two can later collapse into one
// pass over a shared visibility filter.
//
// The caller decides that the profile is an Agent Employee; this pass only
// reads the verdict each command carries. Unrecorded commands carry no
// annotation and are kept, matching Factory.CheckShortcutPrincipal, where an
// unknown verdict dispatches.
//
// Both passes share pruneEmpty, and both rely on running before
// installUnknownSubcommandGuard: pruneEmpty tells a group from a leaf by the
// absence of a RunE, and the guard gives every pure group one.
func pruneForAgentPrincipal(root *cobra.Command) {
	pruneDeniedForAgent(root)
	pruneEmpty(root)
}

func pruneDeniedForAgent(parent *cobra.Command) {
	var toRemove []*cobra.Command
	var toAdd []*cobra.Command
	for _, child := range parent.Commands() {
		if cmdutil.GetAgentSupport(child).Denies() {
			toRemove = append(toRemove, child)
			toAdd = append(toAdd, agentPrincipalStubFrom(child))
			continue
		}
		pruneDeniedForAgent(child)
	}
	if len(toRemove) > 0 {
		parent.RemoveCommand(toRemove...)
		parent.AddCommand(toAdd...)
	}
}

// agentPrincipalStubFrom mirrors strictModeStubFrom, with two differences that
// matter to whoever reads the refusal.
//
// The recovery hint names no command. Strict mode can point at `config
// strict-mode` because the user chose that policy and can leave it; an Agent
// Employee cannot change the credential it carries, so sending it to a config
// command would send it somewhere that cannot help. It gets the same advice the
// run-time refusal gives instead, picking between the two wordings the same way
// CheckAgentEligibility does.
//
// The denial layer is agent_principal rather than strict_mode, so an observer
// reading detail.layer can tell a policy the profile picked from a boundary
// that came with its credential.
func agentPrincipalStubFrom(child *cobra.Command) *cobra.Command {
	stubMessage := cmdutil.AgentRefusalMessage(cmdpolicy.CanonicalPath(child))
	// Only the reason is decided here -- from the markers the mount left on the
	// command -- and cmdutil turns it into the same sentence and advice the
	// run-time gate would have given.
	eligibilityHint := agentRefusalReasonFor(child).Hint()
	denial := cmdpolicy.Denial{
		Layer:        cmdpolicy.LayerAgentPrincipal,
		PolicySource: "agent-support",
		ReasonCode:   "agent_not_supported",
		Reason:       stubMessage,
	}
	// Preserve the original annotations and help text so audit / compliance
	// observers can still see what was withheld, then stamp the denial on top.
	annotations := make(map[string]string, len(child.Annotations)+2)
	for k, v := range child.Annotations {
		annotations[k] = v
	}
	annotations[cmdpolicy.AnnotationDenialLayer] = cmdpolicy.LayerAgentPrincipal
	annotations[cmdpolicy.AnnotationDenialSource] = "agent-support"

	// Args + PersistentPreRunE overrides mirror strictModeStubFrom; see the
	// reasoning there.
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
			hint := recovery.Join("; ",
				recovery.Text(fmt.Sprintf("denied by %s policy (reason_code %s)", cd.Layer, cd.ReasonCode)),
				recovery.Text(eligibilityHint),
			)
			return recovery.Annotate(
				errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", stubMessage).
					WithHint("%s", hint.String()).
					WithCause(cd),
				hint,
			)
		},
	}
}

// agentRefusalReasonFor says why a command is being withheld, reading the
// markers its mount left behind: a shortcut is judged by its recorded verdict,
// which no capability grant can change; a generated method by whether its
// service carries any agent marker at all. The mapping lives here because only
// the tree build can see those markers -- the wording it selects does not.
func agentRefusalReasonFor(cmd *cobra.Command) cmdutil.AgentRefusalReason {
	switch {
	case isShortcutCommand(cmd):
		return cmdutil.AgentRefusedShortcutVerdict
	case cmdutil.AgentServiceAnnotated(cmd):
		return cmdutil.AgentRefusedUnmarkedMethod
	default:
		return cmdutil.AgentRefusedUnmarkedService
	}
}

// isShortcutCommand reports whether the command was mounted as a shortcut, by
// the source marker the mount stamps on it.
func isShortcutCommand(cmd *cobra.Command) bool {
	source, ok := cmdmeta.SourceOf(cmd)
	return ok && source == cmdmeta.SourceShortcut
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
	// strict-mode keeps its short Message + independent Hint and wraps
	// the CommandDeniedError as the Cause by hand; BuildDenialError
	// would override Message with the CommandDeniedError.Error() long
	// form.
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
			hint := recovery.Join("; ",
				recovery.Text(fmt.Sprintf("denied by %s policy (reason_code %s)", cd.Layer, cd.ReasonCode)),
				recovery.Command(recovery.TargetConfigStrictMode, stubHint),
			)
			return recovery.Annotate(
				errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", stubMessage).
					WithHint("%s", hint.String()).
					WithCause(cd),
				hint,
			)
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
