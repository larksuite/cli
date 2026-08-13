// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// contextOptions holds all inputs for the `agents context list|get|delete`
// leaves. A single struct backs all three so the shared fields (Factory, Cmd,
// Ref, As) are wired once; each RunE reads only the fields its verb needs.
type contextOptions struct {
	Factory   *cmdutil.Factory
	Cmd       *cobra.Command
	Ref       string
	CtxID     string
	Params    []string
	Yes       bool
	As        string
	Format    string
	PageSize  int
	PageToken string
}

// NewCmdAgentContext builds the `agents context` command group: manage a remote
// agent's multi-turn contexts (each verb gated on its own capability:
// context_list / context_get / context_delete). It is a pure group with
// no RunE so an unknown subcommand is reported rather than silently swallowed.
func NewCmdAgentContext(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage a remote agent's multi-turn contexts (sessions)",
		Long:  "context list <agent_ref> lists sessions; context get <agent_ref> <ctx-id> shows session detail; context delete <agent_ref> <ctx-id> deletes a session (high-risk, needs --yes).",
	}
	cmd.AddCommand(NewCmdAgentContextList(f))
	cmd.AddCommand(NewCmdAgentContextGet(f))
	cmd.AddCommand(NewCmdAgentContextDelete(f))
	return cmd
}

// NewCmdAgentContextList builds `agents context list <ref>`: enumerate the
// agent's multi-turn contexts into {contexts:[...]} with a meta.count. Risk=read.
func NewCmdAgentContextList(f *cmdutil.Factory) *cobra.Command {
	opts := &contextOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "list <agent_ref>",
		Short: "List a remote agent's multi-turn contexts",
		Long:  "List the multi-turn contexts (sessions) of the agent addressed by agent_ref.",
		Args:  exactArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(opts.Format); err != nil {
				return err
			}
			if err := validatePageSize(opts.PageSize); err != nil {
				return err
			}
			opts.Cmd = cmd
			opts.Ref = args[0]
			return agentContextListRun(opts)
		},
	}
	addPageFlags(cmd, &opts.PageSize, &opts.PageToken)
	addParamFlag(cmd, &opts.Params)
	cmd.Flags().StringVar(&opts.Format, "format", "json", formatFlagHelp)
	cmd.Flags().String("jq", "", "filter the JSON output with a jq expression")
	addAsFlag(cmd, f, &opts.As)
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)
	return cmd
}

// NewCmdAgentContextGet builds `agents context get <ref> <ctx-id>`: fetch a
// single context's detail. Risk=read.
func NewCmdAgentContextGet(f *cmdutil.Factory) *cobra.Command {
	opts := &contextOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "get <agent_ref> <ctx-id>",
		Short: "Show the detail of a single multi-turn context",
		Long:  "Show the detail of the multi-turn context ctx-id under the agent addressed by agent_ref.",
		Args:  exactArgsWithUsage(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(opts.Format); err != nil {
				return err
			}
			opts.Cmd = cmd
			opts.Ref = args[0]
			opts.CtxID = args[1]
			return agentContextGetRun(opts)
		},
	}
	addParamFlag(cmd, &opts.Params)
	cmd.Flags().StringVar(&opts.Format, "format", "json", formatFlagHelp)
	cmd.Flags().String("jq", "", "filter the JSON output with a jq expression")
	addAsFlag(cmd, f, &opts.As)
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)
	return cmd
}

// NewCmdAgentContextDelete builds `agents context delete <ref> <ctx-id>`: destroy
// a multi-turn context. Deletion is irreversible, so it is high-risk-write and
// requires --yes; without it the command returns a confirmation_required error
// (exit 10) before touching the API. Risk=high-risk-write.
func NewCmdAgentContextDelete(f *cmdutil.Factory) *cobra.Command {
	opts := &contextOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "delete <agent_ref> <ctx-id>",
		Short: "Delete a remote agent's multi-turn context (high-risk, needs --yes)",
		Long:  "Delete the multi-turn context ctx-id under the agent addressed by agent_ref. Deletion is irreversible and requires --yes to confirm; otherwise it returns confirmation_required (exit 10).",
		Args:  exactArgsWithUsage(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(opts.Format); err != nil {
				return err
			}
			opts.Cmd = cmd
			opts.Ref = args[0]
			opts.CtxID = args[1]
			return agentContextDeleteRun(opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "confirm the deletion (high-risk; without it the command returns exit 10)")
	addParamFlag(cmd, &opts.Params)
	cmd.Flags().StringVar(&opts.Format, "format", "json", formatFlagHelp)
	cmd.Flags().String("jq", "", "filter the JSON output with a jq expression")
	addAsFlag(cmd, f, &opts.As)
	cmdutil.SetRisk(cmd, cmdutil.RiskHighRiskWrite)
	return cmd
}

// agentContextListRun runs `context list`: resolves the provider, lists
// contexts in the provider's most-recent-first order, and emits {contexts:[...]}
// with meta.count through content-safety scanning (the rollup is derived from
// untrusted agent activity).
func agentContextListRun(opts *contextOptions) error {
	f := opts.Factory
	_, spec, agentID, id, err := resolveSpec(f, opts.Cmd, opts.Ref, opts.As)
	if err != nil {
		return err
	}
	// Whole-agent brand gate FIRST (offline): a brand-hidden agent reports
	// unavailable_for_brand uniformly for every verb — even one it does not wire —
	// so it must precede the capability nil-gate below.
	if err := brandGate(f, spec, opts.Ref); err != nil {
		return err
	}
	// Capability gate BEFORE the client: context_list is derived from ListContexts
	// being wired, so a spec without it returns unsupported_capability offline.
	if spec.ListContexts.Handler == nil {
		return capabilityError(opts.Ref, "context list", iagents.CapContextList)
	}
	// Per-capability brand gate: applies only to a wired op.
	if err := opBrandGate(f, spec.ListContexts.Brands, opts.Ref, "context list"); err != nil {
		return err
	}
	vp, err := validateParams(opts.Params, spec.ListContexts.Params, iagents.VerbContextList, spec, opts.Ref)
	if err != nil {
		return err
	}
	rt, err := runtimeFor(f, id, agentID, vp.Resolved)
	if err != nil {
		return err
	}
	// Local scope preflight: after runtimeFor, before the API call.
	if err := preflightScopesForRef(f, id, opts.Ref); err != nil {
		return err
	}
	contexts, pageInfo, err := spec.ListContexts.Handler(opts.Cmd.Context(), rt,
		iagents.PageParams{Token: opts.PageToken, Size: opts.PageSize})
	if err != nil {
		return err
	}
	// Ordering is the provider's contract (most-recent-first), consistent across
	// and within pages — the CLI does not re-sort a page.
	if contexts == nil {
		contexts = []iagents.ContextSummary{} // always emit [] not null (matches the Card.Parameters array convention)
	}
	return scanAndEmitData(f, opts.Cmd, opts.Format,
		map[string]interface{}{"contexts": contexts},
		listMetaPage(len(contexts), pageInfo, contextListNext(opts, f, pageInfo)),
		func(w io.Writer) { printContextsTSV(w, contexts) })
}

// contextListNext builds the next-page action for `context list`, replaying the
// caller's ref with the returned cursor. The ref is gated by safeNextRef; a
// failing ref drops the action (the cursor still rides meta.page_token as data).
func contextListNext(opts *contextOptions, f *cmdutil.Factory, info iagents.PageInfo) []output.NextAction {
	if !safeNextRef(opts.Ref) {
		return nil
	}
	next := nextPageAction(fmt.Sprintf("lark-cli agents context list %s", opts.Ref), opts.PageSize, info)
	carryAsIntoNext(opts.Cmd, f, next)
	return next
}

// agentContextGetRun runs `context get`: resolves the provider, fetches the
// context detail (metadata + rollup + the single active_task, NOT the full task
// list), derives the active task's IsTerminal, and emits it through
// content-safety scanning (active_task.Summary is untrusted agent text).
func agentContextGetRun(opts *contextOptions) error {
	f := opts.Factory
	_, spec, agentID, id, err := resolveSpec(f, opts.Cmd, opts.Ref, opts.As)
	if err != nil {
		return err
	}
	// Whole-agent brand gate FIRST (offline): a brand-hidden agent reports
	// unavailable_for_brand uniformly for every verb — even one it does not wire —
	// so it must precede the capability nil-gate below.
	if err := brandGate(f, spec, opts.Ref); err != nil {
		return err
	}
	// Capability gate BEFORE the client.
	if spec.GetContext.Handler == nil {
		return capabilityError(opts.Ref, "context get", iagents.CapContextGet)
	}
	// Per-capability brand gate: applies only to a wired op.
	if err := opBrandGate(f, spec.GetContext.Brands, opts.Ref, "context get"); err != nil {
		return err
	}
	vp, err := validateParams(opts.Params, spec.GetContext.Params, iagents.VerbContextGet, spec, opts.Ref)
	if err != nil {
		return err
	}
	rt, err := runtimeFor(f, id, agentID, vp.Resolved)
	if err != nil {
		return err
	}
	// Local scope preflight: after runtimeFor, before the API call.
	if err := preflightScopesForRef(f, id, opts.Ref); err != nil {
		return err
	}
	detail, err := spec.GetContext.Handler(opts.Cmd.Context(), rt, opts.CtxID)
	if err != nil {
		return err
	}
	if detail != nil && detail.ActiveTask != nil {
		// Derive IsTerminal from State (single source of truth) for the active task
		// summary before emission — the provider only fills State.
		detail.ActiveTask.IsTerminal = detail.ActiveTask.State.IsTerminal()
	}
	return scanAndEmitData(f, opts.Cmd, opts.Format, detail, nil,
		func(w io.Writer) { printContextDetailPretty(w, detail) })
}

// agentContextDeleteRun runs `context delete`. The --yes confirmation guard runs
// first so a missing confirmation returns confirmation_required (exit 10) before
// any provider is built and holds even under a nil Factory. Only a
// confirmed delete reaches resolveSpec + DeleteContext.
func agentContextDeleteRun(opts *contextOptions) error {
	if !opts.Yes {
		// Not the generic English RequireConfirmation: deletion is the most
		// destructive gate in the agent tree, so the message must state the
		// irreversible blast radius in the same voice (Chinese, self-contained)
		// as the other two exit-10 gates.
		return errs.NewConfirmationRequiredError(errs.RiskHighRiskWrite, "agents context delete",
			"deleting a context irreversibly removes it and every task record under it").
			WithHint("add --yes to confirm the deletion")
	}

	f := opts.Factory
	_, spec, agentID, id, err := resolveSpec(f, opts.Cmd, opts.Ref, opts.As)
	if err != nil {
		return err
	}
	// Whole-agent brand gate FIRST (offline): a brand-hidden agent reports
	// unavailable_for_brand uniformly for every verb — even one it does not wire —
	// so it must precede the capability nil-gate below.
	if err := brandGate(f, spec, opts.Ref); err != nil {
		return err
	}
	// Capability gate BEFORE the client.
	if spec.DeleteContext.Handler == nil {
		return capabilityError(opts.Ref, "context delete", iagents.CapContextDelete)
	}
	// Per-capability brand gate: applies only to a wired op.
	if err := opBrandGate(f, spec.DeleteContext.Brands, opts.Ref, "context delete"); err != nil {
		return err
	}
	vp, err := validateParams(opts.Params, spec.DeleteContext.Params, iagents.VerbContextDelete, spec, opts.Ref)
	if err != nil {
		return err
	}
	rt, err := runtimeFor(f, id, agentID, vp.Resolved)
	if err != nil {
		return err
	}
	// Local scope preflight: after runtimeFor, before the API call.
	if err := preflightScopesForRef(f, id, opts.Ref); err != nil {
		return err
	}
	if err := spec.DeleteContext.Handler(opts.Cmd.Context(), rt, opts.CtxID); err != nil {
		return err
	}
	// pretty is a human view only; a --jq expression implies structured JSON.
	if opts.Format == "pretty" && jqExpr(opts.Cmd) == "" {
		fmt.Fprintf(f.IOStreams.Out, "context_id: %s\ndeleted: true\n", kvValue(opts.CtxID))
		return nil
	}
	env := output.Envelope{
		OK:       true,
		Identity: string(id),
		Data:     map[string]interface{}{"context_id": opts.CtxID, "deleted": true},
		Notice:   output.GetNotice(),
	}
	if jq := jqExpr(opts.Cmd); jq != "" {
		return output.JqFilter(f.IOStreams.Out, env, jq)
	}
	output.PrintJson(f.IOStreams.Out, env)
	return nil
}
