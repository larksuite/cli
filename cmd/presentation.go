// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	configcmd "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/surface"
)

const annotationUnavailableMessage = "lark:presentation_unavailable_message"

type projectedCommand struct {
	state  surface.CommandState
	denial cmdpolicy.Denial
}

// presentationProjection keeps one distribution's build-time presentation
// state and denial provenance together, so a concealed command retains the
// cause installed on its unavailable projection.
type presentationProjection struct {
	commands map[surface.CommandID]projectedCommand
}

func newPresentationProjection(denied map[string]cmdpolicy.Denial) *presentationProjection {
	projection := &presentationProjection{
		commands: make(map[surface.CommandID]projectedCommand, len(denied)),
	}
	for path, denial := range denied {
		projection.commands[surface.CommandID(path)] = projectedCommand{
			state:  surface.CommandDeniedVisible,
			denial: denial,
		}
	}
	return projection
}

func (p *presentationProjection) recordConcealed(path string, denial cmdpolicy.Denial) {
	p.commands[surface.CommandID(path)] = projectedCommand{
		state:  surface.CommandConcealed,
		denial: denial,
	}
}

func (p *presentationProjection) denial(path string) (cmdpolicy.Denial, bool) {
	command, ok := p.commands[surface.CommandID(path)]
	if !ok || command.state != surface.CommandConcealed {
		return cmdpolicy.Denial{}, false
	}
	return command.denial, true
}

func (p *presentationProjection) plan() *surface.Plan {
	states := make(map[surface.CommandID]surface.CommandState, len(p.commands))
	for id, command := range p.commands {
		states[id] = command.state
	}
	return surface.NewPlan(states)
}

func (p *presentationProjection) hasConcealedCommands() bool {
	for _, command := range p.commands {
		if command.state == surface.CommandConcealed {
			return true
		}
	}
	return false
}

// applyDistributionPresentation projects enforcement decisions onto the
// command surface of this one build. Enforcement has already installed its
// policy-rich deny stubs. Without an explicit presentation option, those stubs
// and their legacy help/completion behavior are left untouched.
func applyDistributionPresentation(
	root *cobra.Command,
	cfg restrictionPresentationConfig,
	denied map[string]cmdpolicy.Denial,
) (*surface.Plan, bool) {
	projection := newPresentationProjection(denied)
	if !cfg.enabled {
		return projection.plan(), false
	}

	collectPluginConcealments(root, denied, projection)
	if cfg.hidePolicyDiagnostics {
		collectDiagnosticConcealments(root, projection)
	}
	propagateConcealedPureGroups(root, projection)

	installUnavailableProjections(root, projection, cfg.effectiveUnavailableMessage())

	plan := projection.plan()
	applyPresentationAffordances(root, plan)
	return plan, projection.hasConcealedCommands()
}

func collectPluginConcealments(
	root *cobra.Command,
	denied map[string]cmdpolicy.Denial,
	projection *presentationProjection,
) {
	for path, denial := range denied {
		if !cmdpolicy.IsPluginPolicySource(denial.PolicySource) {
			continue
		}
		cmd := findByPath(root, path)
		if cmd == nil || commandDenialLayer(cmd) == cmdpolicy.LayerStrictMode {
			continue
		}
		projection.recordConcealed(path, denial)
	}
}

func collectDiagnosticConcealments(
	root *cobra.Command,
	projection *presentationProjection,
) {
	for _, path := range cmdpolicy.DiagnosticPaths() {
		if findByPath(root, path) == nil {
			continue
		}
		projection.recordConcealed(path, cmdpolicy.Denial{
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "distribution:presentation",
			ReasonCode:   "diagnostics_concealed",
			Reason:       "policy diagnostics concealed by the distribution",
		})
	}
}

func propagateConcealedPureGroups(
	root *cobra.Command,
	projection *presentationProjection,
) {
	// A pure parent becomes absent only when every live child is absent. Repeat
	// bottom-up until all newly-empty intermediate groups converge.
	for {
		changed := false
		plan := projection.plan()
		walkCommandsPostOrder(root, func(cmd *cobra.Command) {
			path, denial, ok := concealedPureGroup(cmd, plan, projection)
			if !ok {
				return
			}
			projection.recordConcealed(path, denial)
			changed = true
		})
		if !changed {
			break
		}
	}
}

func concealedPureGroup(
	cmd *cobra.Command,
	plan *surface.Plan,
	projection *presentationProjection,
) (string, cmdpolicy.Denial, bool) {
	path := cmdpolicy.CanonicalPath(cmd)
	if !cmd.HasParent() || !isPresentationPureGroup(cmd) ||
		plan.IsConcealed(surface.CommandID(path)) {
		return "", cmdpolicy.Denial{}, false
	}
	children := cmd.Commands()
	if len(children) == 0 {
		return "", cmdpolicy.Denial{}, false
	}

	var cause cmdpolicy.Denial
	for _, child := range children {
		childPath := cmdpolicy.CanonicalPath(child)
		if !plan.IsConcealed(surface.CommandID(childPath)) {
			return "", cmdpolicy.Denial{}, false
		}
		if denial, ok := projection.denial(childPath); ok && cause.Layer == "" {
			cause = denial
		}
	}
	if cause.Layer == "" {
		cause = cmdpolicy.Denial{
			Layer:        cmdpolicy.LayerPolicy,
			PolicySource: "distribution:presentation",
			ReasonCode:   "all_children_concealed",
			Reason:       "all child commands are concealed",
		}
	}
	return path, cause, true
}

func installUnavailableProjections(
	root *cobra.Command,
	projection *presentationProjection,
	message string,
) {
	for id, command := range projection.commands {
		if command.state != surface.CommandConcealed {
			continue
		}
		path := string(id)
		if cmd := findByPath(root, path); cmd != nil {
			installUnavailableProjection(cmd, path, command.denial, message)
		}
	}
}

func applyPresentationAffordances(root *cobra.Command, plan *surface.Plan) {
	applyPluginFlagGate(root, plan)
	configcmd.ProjectInitHelp(
		findByPath(root, string(surface.CommandConfigInit)),
		plan.CanReference(surface.CommandConfigBind),
	)
	root.Long = renderRootHelpSections(rootLongSections, plan)
	root.SetUsageTemplate(renderRootUsageTemplate(plan))
}

func commandDenialLayer(cmd *cobra.Command) string {
	if cmd == nil || cmd.Annotations == nil {
		return ""
	}
	return cmd.Annotations[cmdpolicy.AnnotationDenialLayer]
}

func isPresentationPureGroup(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	return (cmd.Run == nil && cmd.RunE == nil) || cmdpolicy.IsPureGroup(cmd)
}

func walkCommandsPostOrder(cmd *cobra.Command, visit func(*cobra.Command)) {
	for _, child := range cmd.Commands() {
		walkCommandsPostOrder(child, visit)
	}
	visit(cmd)
}

// installUnavailableProjection changes presentation only. It preserves the
// enforcement denial as the in-process cause when one exists, while the wire
// intentionally exposes no policy source, rule name, or reason code.
func installUnavailableProjection(cmd *cobra.Command, path string, denial cmdpolicy.Denial, message string) {
	cmd.Hidden = true
	cmd.DisableFlagParsing = true
	cmd.Args = cobra.ArbitraryArgs
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		c.SilenceUsage = true
		return nil
	}
	cmd.PersistentPreRun = nil
	cmd.PreRunE = nil
	cmd.PreRun = nil

	hideFlags := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(flag *pflag.Flag) {
			flag.Hidden = true
		})
	}
	// Hide only flags owned by this command. cmd.Flags() may contain inherited
	// flag pointers after Cobra merges sets; mutating those would hide a global
	// flag from unrelated commands.
	hideFlags(cmd.LocalNonPersistentFlags())
	hideFlags(cmd.PersistentFlags())
	cmd.ValidArgs = nil
	cmd.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationUnavailableMessage] = message
	if cmd.Annotations[cmdpolicy.AnnotationDenialLayer] == "" {
		cmd.Annotations[cmdpolicy.AnnotationDenialLayer] = denial.Layer
		cmd.Annotations[cmdpolicy.AnnotationDenialSource] = denial.PolicySource
	}

	cmd.RunE = func(*cobra.Command, []string) error {
		err := errs.NewValidationError(errs.SubtypeCommandUnavailable, "%s", message)
		if denial.Layer != "" {
			err.WithCause(cmdpolicy.CommandDeniedFromDenial(path, denial))
		}
		return err
	}
	cmd.Run = nil
}

// unavailableHelpMessage is deliberately keyed only by the opt-in projection
// annotation. A legacy Restrict denial carries enforcement annotations but
// continues to use Cobra's stock explicit-help behavior.
func unavailableHelpMessage(cmd *cobra.Command) (string, bool) {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Annotations == nil {
			continue
		}
		if message := current.Annotations[annotationUnavailableMessage]; message != "" {
			return message, true
		}
	}
	return "", false
}

// findByPath resolves a canonical slash path (for example
// "config/policy/show") to a command node.
func findByPath(root *cobra.Command, path string) *cobra.Command {
	cur := root
	for _, segment := range strings.Split(path, "/") {
		var next *cobra.Command
		for _, child := range cur.Commands() {
			if child.Name() == segment {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}
