// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// providerInfo describes a registered provider adapter in `agents list` output.
// Every field is sourced from the registered iagents.Provider (the single
// source of truth).
type providerInfo struct {
	Scheme         string `json:"scheme"`
	Label          string `json:"label"`
	AgentRefFormat string `json:"agent_ref_format"`
	Kind           string `json:"kind"`
	AgentIDSource  string `json:"agent_id_source"`
	// ListParams documents the business parameters `agents list <scheme>` itself
	// takes — surfaced HERE (the offline, always-reachable provider listing)
	// because at list time the caller holds no agent_ref yet, so a card-based
	// hint would point at an unreachable road.
	ListParams []iagents.CardParam `json:"list_parameters,omitempty"`
}

// listOptions holds all inputs for `agents list [scheme]`.
type listOptions struct {
	Factory   *cmdutil.Factory
	Cmd       *cobra.Command
	Scheme    string
	Params    []string
	Format    string
	As        string
	PageSize  int
	PageToken string
}

// NewCmdAgentList builds `agents list [scheme]`. Without an argument it
// enumerates the registered provider adapters with their metadata — a
// pure, API-free listing. With a scheme it performs second-level discovery:
// catalog providers enumerate offline from their static set; instance providers
// enumerate via their optional ListAgents hook (absent ⇒ unsupported_capability
// with the agent_id_source guidance). Risk=read.
func NewCmdAgentList(f *cmdutil.Factory) *cobra.Command {
	opts := &listOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "list [scheme]",
		Short: "List registered agent providers, or enumerate the agents under one provider",
		Long:  "With no argument, list the built-in provider adapters and their metadata (label / agent_ref format / kind / how to obtain an agent_id) without calling any API. With a scheme, enumerate the agents under that provider (catalog providers must be enumerable; instance providers may not support it).",
		Args:  maximumArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(opts.Format); err != nil {
				return err
			}
			if err := validatePageSize(opts.PageSize); err != nil {
				return err
			}
			opts.Cmd = cmd
			if len(args) == 1 {
				opts.Scheme = args[0]
			}
			return agentListRun(opts)
		},
	}
	// --page-size / --page-token apply only to the instance enumeration path
	// (prov.ListAgents); the offline catalog listing and the no-scheme provider
	// listing ignore them.
	addPageFlags(cmd, &opts.PageSize, &opts.PageToken)
	addParamFlag(cmd, &opts.Params)
	cmd.Flags().StringVar(&opts.Format, "format", "json", formatFlagHelp)
	cmd.Flags().String("jq", "", "filter the JSON output with a jq expression")
	// --as only matters for the online `list <scheme>` enumeration (an instance
	// provider's ListAgents call); the no-scheme provider listing is offline and
	// identity-independent, so it ignores --as.
	addAsFlag(cmd, f, &opts.As)
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)
	return cmd
}

// agentListRun dispatches `agents list [scheme]`: with a scheme it lists that
// provider's agents (second-level discovery); without it renders the provider
// listing. JSON envelope is the default; `pretty` is the opt-in human view.
func agentListRun(opts *listOptions) error {
	if opts.Scheme != "" {
		return agentListSchemeRun(opts)
	}
	// The no-scheme form is a pure offline registry listing — business params
	// have no target operation, so reject explicitly rather than silently
	// ignoring what the caller thought they were passing.
	if len(opts.Params) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--param only means something with agents list <scheme> (the bare listing is a purely local enumeration)").
			WithParam("--param").
			WithHint("add a scheme and resend, e.g. lark-cli agents list <scheme> --param k=v; each provider's list parameters are in the list_parameters field of this command's output")
	}

	f := opts.Factory
	providers := listProviders()

	// pretty is a human view only; a --jq expression implies structured JSON.
	if opts.Format == "pretty" && jqExpr(opts.Cmd) == "" {
		fmt.Fprintf(f.IOStreams.Out, "SCHEME\tLABEL\tAGENT_REF_FORMAT\tKIND\n")
		for _, p := range providers {
			fmt.Fprintf(f.IOStreams.Out, "%s\t%s\t%s\t%s\n", p.Scheme, p.Label, p.AgentRefFormat, p.Kind)
		}
		// agent_id_source is a full sentence — a TSV column would blow out the
		// row width, so surface it as a per-provider footer instead. This is the
		// single most important "where do I get an agent_id" cue for newcomers
		// and must not vanish in the human-readable view.
		fmt.Fprintln(f.IOStreams.Out)
		for _, p := range providers {
			fmt.Fprintf(f.IOStreams.Out, "agent_id source (%s): %s\n", p.Scheme, p.AgentIDSource)
		}
		return nil
	}

	env := output.Envelope{
		OK:     true,
		Data:   map[string]interface{}{"providers": providers},
		Meta:   listMeta(len(providers)),
		Notice: output.GetNotice(),
	}
	if jq := jqExpr(opts.Cmd); jq != "" {
		return output.JqFilter(f.IOStreams.Out, env, jq)
	}
	output.PrintJson(f.IOStreams.Out, env)
	return nil
}

// agentListSchemeRun runs `agents list <scheme>`: second-level enumeration for
// one provider. A catalog provider enumerates OFFLINE from its static set
// (prov.ListCatalog). An instance provider enumerates ONLINE via its optional
// ListAgents hook (needs a configured client); an instance provider without that
// hook is not enumerable and returns unsupported_capability + the AgentIDSource
// hint — surfaced before the client is built.
func agentListSchemeRun(opts *listOptions) error {
	f := opts.Factory
	prov, ok := iagents.Info(opts.Scheme)
	if !ok {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown agent provider '%s', currently registered: %s",
			opts.Scheme, iagents.KnownSchemes()).
			WithHint("run lark-cli agents list to see the available providers")
	}

	var agents []iagents.AgentSummary
	var identity string           // set only on the online (instance) path, which resolves one
	var pageInfo iagents.PageInfo // set only on the online (instance) path
	catalog := prov.Kind() == iagents.KindCatalog
	if catalog {
		// Offline catalog enumeration takes no business params (ListParams
		// requires a ListAgents hook); validate against the empty set so a stray
		// --param is rejected with the same teaching error instead of ignored.
		// The catalog set is finite and offline, so it is UNPAGED: --page-size /
		// --page-token are ignored on this path (documented on the command).
		if _, err := validateListParams(opts.Params, nil, opts.Scheme); err != nil {
			return err
		}
		agents = prov.ListCatalog(resolvedBrand(opts.Factory)) // offline, brand-filtered
	} else {
		// instance: needs the online ListAgents hook. Absent ⇒ not enumerable.
		if prov.ListAgents == nil {
			return errs.NewValidationError(errs.SubtypeUnsupportedCapability,
				"provider '%s' does not support listing agents", opts.Scheme).
				WithHint("%s", prov.AgentIDSource)
		}
		// --page-size is validated uniformly in RunE (alongside validateFormat), so
		// this paginated path does not re-check it here.
		// Enumeration is a real online call with no agent_id, so it runs the same
		// gates every ref-addressed online verb runs (via resolveSpec +
		// preflightScopesForRef): the global user|bot whitelist, the provider's
		// identity subset, and the all-or-nothing scope preflight — keyed on the
		// scheme since there is no ref.
		// agentID is empty (enumeration is not scoped to a single agent).
		id := f.ResolveAs(opts.Cmd.Context(), opts.Cmd, core.Identity(opts.As))
		if err := f.CheckIdentity(id, supportedIdentities); err != nil {
			return err
		}
		if err := checkProviderIdentity(f, id, prov); err != nil {
			return err
		}
		identity = string(id)
		// list is a provider-level operation: params validate against ListParams
		// (no spec, so no cross-operation reverse lookup); the error hint points
		// at `agents list` output's list_parameters, not at an agent card the
		// caller cannot address yet (it holds no agent_ref at list time).
		vp, err := validateListParams(opts.Params, prov.ListParams, opts.Scheme)
		if err != nil {
			return err
		}
		rt, err := runtimeFor(f, id, "", vp.Resolved)
		if err != nil {
			return err
		}
		if err := preflightScopesForScheme(f, id, opts.Scheme); err != nil {
			return err
		}
		agents, pageInfo, err = prov.ListAgents(opts.Cmd.Context(), rt,
			iagents.PageParams{Token: opts.PageToken, Size: opts.PageSize})
		if err != nil {
			return err
		}
	}
	if agents == nil {
		agents = []iagents.AgentSummary{} // always emit [] not null
	}

	// pretty is a human view only; a --jq expression implies structured JSON.
	if opts.Format == "pretty" && jqExpr(opts.Cmd) == "" {
		// Name/Description are agent-controlled remote strings — ANSI-strip
		// them before writing to the terminal.
		fmt.Fprintf(f.IOStreams.Out, "AGENT_REF\tNAME\tDESCRIPTION\n")
		for _, a := range agents {
			fmt.Fprintf(f.IOStreams.Out, "%s\t%s\t%s\n", stripANSI(a.AgentRef), stripANSI(a.Name), stripANSI(a.Description))
		}
		return nil
	}

	// Catalog is unpaged (plain count); the instance path carries has_more /
	// page_token and a next-page action when there are more agents.
	meta := listMeta(len(agents))
	if !catalog {
		meta = listMetaPage(len(agents), pageInfo, listSchemeNext(opts, f, pageInfo))
	}
	env := output.Envelope{
		OK:       true,
		Identity: identity, // empty for the offline catalog path (omitempty)
		Data:     map[string]interface{}{"agents": agents},
		Meta:     meta,
		Notice:   output.GetNotice(),
	}
	if jq := jqExpr(opts.Cmd); jq != "" {
		return output.JqFilter(f.IOStreams.Out, env, jq)
	}
	output.PrintJson(f.IOStreams.Out, env)
	return nil
}

// listSchemeNext builds the next-page action for the instance `list <scheme>`
// enumeration, replaying the scheme with the returned cursor. The scheme is
// gated by safeNextID (no colon, so safeNextRef does not apply); a failing scheme
// drops the action (the cursor still rides meta.page_token as data).
func listSchemeNext(opts *listOptions, f *cmdutil.Factory, info iagents.PageInfo) []output.NextAction {
	if !safeNextID(opts.Scheme) {
		return nil
	}
	next := nextPageAction(fmt.Sprintf("lark-cli agents list %s", opts.Scheme), opts.PageSize, info)
	carryAsIntoNext(opts.Cmd, f, next)
	return next
}

// listProviders builds the provider descriptors from the built-in registry so
// the listing stays in sync with whatever adapters are registered.
func listProviders() []providerInfo {
	schemes := iagents.RegisteredSchemes()
	out := make([]providerInfo, 0, len(schemes))
	for _, s := range schemes {
		// s comes from RegisteredSchemes, so Info always succeeds.
		prov, _ := iagents.Info(s)
		out = append(out, providerInfo{
			Scheme:         s,
			Label:          prov.Label,
			AgentRefFormat: prov.AgentRefFormat(),
			Kind:           string(prov.Kind()),
			AgentIDSource:  prov.AgentIDSource,
			ListParams:     prov.ListParams,
		})
	}
	return out
}
