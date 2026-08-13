// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// cardOptions holds all inputs for `agents card <ref>`.
type cardOptions struct {
	Factory   *cmdutil.Factory
	Cmd       *cobra.Command
	Ref       string
	Operation string
	As        string
	Format    string
}

// verbCommandTemplate maps each operation verb to the human command that
// executes it — surfaced in `--operation` output so the verb↔command mapping
// is a lookup, not something the caller memorizes (artifact_download being the
// one non-obvious row). Templates carry <...> placeholders and are never
// executable verbatim.
var verbCommandTemplate = map[string]string{
	iagents.VerbSend:             "lark-cli agents send <agent_ref> --text <text> [--param k=v ...]",
	iagents.VerbTaskGet:          "lark-cli agents task get <agent_ref> <task-id> [--watch --timeout 90s] [--param k=v ...]",
	iagents.VerbTaskList:         "lark-cli agents task list <agent_ref> [--context-id <ctx-id>] [--param k=v ...]",
	iagents.VerbTaskCancel:       "lark-cli agents task cancel <agent_ref> <task-id> [--param k=v ...]",
	iagents.VerbContextList:      "lark-cli agents context list <agent_ref> [--param k=v ...]",
	iagents.VerbContextGet:       "lark-cli agents context get <agent_ref> <ctx-id> [--param k=v ...]",
	iagents.VerbContextDelete:    "lark-cli agents context delete <agent_ref> <ctx-id> --yes [--param k=v ...]",
	iagents.VerbArtifactDownload: "lark-cli agents task get <agent_ref> <task-id> --artifact <artifact-id> -o <output> [--param k=v ...]",
}

// NewCmdAgentCard builds `agents card <ref>`: show an agent's capability card
// (lean by default: capabilities + has_parameters), or — with --operation —
// one operation's full parameter contract (--operation all returns every
// operation at once). Resolution is offline; Describe enrichment is
// best-effort when a client is configured. Risk=read.
func NewCmdAgentCard(f *cmdutil.Factory) *cobra.Command {
	opts := &cardOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "card <agent_ref>",
		Short: "Show a remote agent's capability card, or one operation's parameter contract",
		Long: "Fetch and show an agent's capability card. The default card is lean: capabilities decide which verbs are available, " +
			"has_parameters lists the verbs that need a parameter lookup. Use --operation <verb> to fetch one operation's full parameter " +
			"contract (name/type/required/enum/default + the command shape), or --operation all for every operation at once.",
		Args: exactArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(opts.Format); err != nil {
				return err
			}
			opts.Cmd = cmd
			opts.Ref = args[0]
			return agentCardRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.Operation, "operation", "", "query one operation's parameter contract: a verb (a capabilities key, or send), or all to fetch every one at once")
	cmd.Flags().StringVar(&opts.Format, "format", "json", formatFlagHelp)
	cmd.Flags().String("jq", "", "filter the JSON output with a jq expression")
	if f != nil {
		cmdutil.AddAPIIdentityFlag(cmd.Context(), cmd, f, &opts.As)
	} else {
		// f is nil only in construction-time unit tests; register a bare --as so
		// the flag surface is still assertable without a Factory.
		cmd.Flags().StringVar(&opts.As, "as", "", "identity type: user | bot (the identities an agent actually supports are listed by agents card)")
	}
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)
	return cmd
}

// agentCardRun resolves the provider addressed by ref and emits either the
// lean capability card or (--operation) a parameter-contract subquery. The
// card is first-party static data (not agent-generated content), so it
// bypasses content-safety scanning. The JSON success envelope is the default;
// --format pretty opts into the human-readable listing; --jq forces JSON.
func agentCardRun(opts *cardOptions) error {
	f := opts.Factory
	// Resolution is fully offline (no client), so `agents card` works before
	// config init. The capability matrix + static metadata are always available.
	prov, spec, agentID, id, err := resolveSpecForCard(f, opts.Cmd, opts.Ref, opts.As)
	if err != nil {
		return err
	}
	// Whole-agent brand gate (offline): an agent hidden from the current brand
	// has no card to show under it.
	if err := brandGate(f, spec, opts.Ref); err != nil {
		return err
	}

	if opts.Operation != "" {
		return agentCardOperationRun(opts, prov, spec, id)
	}

	// Best-effort remote enrichment: if a client is configured, pass a runtime so
	// a provider's Describe can fill Name/Description from the platform; otherwise
	// rt stays nil and BuildCard returns the offline (caps + static) card. An
	// unsupported current identity also keeps the static card available: the card
	// itself is how callers discover the provider's supported identity subset.
	var rt iagents.Runtime
	if providerSupportsIdentity(prov, id) {
		if r, rerr := runtimeFor(f, id, agentID, nil); rerr == nil {
			rt = r
		}
	}
	card := iagents.BuildCard(opts.Cmd.Context(), prov, spec, agentID, resolvedBrand(f), rt)

	jq := jqExpr(opts.Cmd)
	// pretty is a human view only; a --jq expression implies structured JSON,
	// so it takes precedence over the pretty format.
	if opts.Format == "pretty" && jq == "" {
		printCardPretty(f.IOStreams.Out, card)
		return nil
	}

	env := output.Envelope{
		OK:       true,
		Identity: string(id),
		Data:     card,
		Notice:   output.GetNotice(),
	}
	if jq != "" {
		return output.JqFilter(f.IOStreams.Out, env, jq)
	}
	output.PrintJson(f.IOStreams.Out, env)
	return nil
}

// operationContract is one operation's parameter contract in `card
// --operation` output. Parameters is always an array (empty is [], never
// null); Command is the human command shape (a template, never executable
// verbatim) and is omitted for unwired operations.
type operationContract struct {
	Operation  string              `json:"operation"`
	Supported  bool                `json:"supported"`
	Command    string              `json:"command,omitempty"`
	Parameters []iagents.CardParam `json:"parameters"`
	// ParametersSource is "template" on instance providers (both the single-verb
	// and the all forms), mirroring the lean card's honesty label.
	ParametersSource string `json:"parameters_source,omitempty"`
}

// contractFor projects one OpInfo into its output contract.
func contractFor(o iagents.OpInfo) operationContract {
	c := operationContract{Operation: o.Verb, Supported: o.Wired, Parameters: []iagents.CardParam{}}
	if o.Wired {
		c.Command = verbCommandTemplate[o.Verb]
		if o.Params != nil {
			c.Parameters = o.Params
		}
	}
	return c
}

// agentCardOperationRun serves `card --operation <verb|all>`: the parameter
// contract subquery. Everything is offline static data. Edge behaviors are
// deterministic: an unknown verb is invalid_argument listing the vocabulary;
// an unwired verb answers supported:false; a wired zero-param verb answers
// supported:true + parameters:[] ("nothing to pass" — not "not found").
func agentCardOperationRun(opts *cardOptions, prov iagents.Provider, spec *iagents.AgentSpec, id core.Identity) error {
	f := opts.Factory
	verb := opts.Operation

	var data any
	var prettyFn func(io.Writer)

	if verb == "all" {
		all := map[string]operationContract{}
		for _, o := range spec.Ops() {
			all[o.Verb] = contractFor(o)
		}
		if prov.Kind() == iagents.KindInstance {
			data = map[string]any{"operations": all, "parameters_source": "template"}
		} else {
			data = map[string]any{"operations": all}
		}
		prettyFn = func(w io.Writer) {
			for _, o := range spec.Ops() {
				printOperationPretty(w, contractFor(o))
			}
		}
	} else {
		o, ok := spec.Op(verb)
		if !ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unknown operation %q, valid values: %s, all", verb, strings.Join(iagents.Verbs(), ", ")).
				WithParam("--operation").
				WithHint("the valid --operation verbs are the ones listed in the message (the 8 operation names; file_input/input_required in capabilities are behavior flags, not verbs); all fetches every one at once")
		}
		c := contractFor(o)
		if prov.Kind() == iagents.KindInstance {
			c.ParametersSource = "template" // struct reuse: never invent a command:"" key for an unwired operation
		}
		data = c
		prettyFn = func(w io.Writer) { printOperationPretty(w, c) }
	}

	if opts.Format == "pretty" && jqExpr(opts.Cmd) == "" {
		prettyFn(f.IOStreams.Out)
		return nil
	}
	env := output.Envelope{
		OK:       true,
		Identity: string(id),
		Data:     data,
		Notice:   output.GetNotice(),
	}
	if jq := jqExpr(opts.Cmd); jq != "" {
		return output.JqFilter(f.IOStreams.Out, env, jq)
	}
	output.PrintJson(f.IOStreams.Out, env)
	return nil
}

// printOperationPretty renders one operation contract as a human block.
func printOperationPretty(w io.Writer, c operationContract) {
	if !c.Supported {
		fmt.Fprintf(w, "operation: %s (unsupported)\n", c.Operation)
		return
	}
	fmt.Fprintf(w, "operation: %s\n", c.Operation)
	if c.Command != "" {
		fmt.Fprintf(w, "  command: %s\n", c.Command)
	}
	if len(c.Parameters) == 0 {
		fmt.Fprintln(w, "  parameters: (none)")
		return
	}
	fmt.Fprintln(w, "  parameters:")
	for _, p := range c.Parameters {
		printParamPretty(w, p)
	}
}

// printParamPretty renders one declaration: the familiar "name: type
// (required) — desc" first line plus an attribute line (enum / range /
// default) when present. Desc/enum are provider-authored strings → stripANSI.
func printParamPretty(w io.Writer, p iagents.CardParam) {
	req := ""
	if p.Required {
		req = " (required)"
	}
	fmt.Fprintf(w, "    %s: %s%s", p.Name, p.Type, req)
	if p.Desc != "" {
		fmt.Fprintf(w, " — %s", stripANSI(p.Desc))
	}
	fmt.Fprintln(w)
	var attrs []string
	if len(p.Enum) > 0 {
		attrs = append(attrs, "values: "+stripANSI(strings.Join(p.Enum, " | ")))
	}
	if p.Min != nil || p.Max != nil {
		attrs = append(attrs, "range: "+rangePretty(p))
	}
	if p.Default != "" {
		attrs = append(attrs, "default: "+stripANSI(p.Default))
	}
	if p.NoCarry {
		attrs = append(attrs, "not carried in meta.next (give a fresh value per call)")
	}
	if len(attrs) > 0 {
		fmt.Fprintf(w, "        %s\n", strings.Join(attrs, " · "))
	}
	// object: render each leaf indented so the dotted-path form is visible
	for _, f := range p.Fields {
		leaf := f
		leaf.Name = p.Name + "." + f.Name
		fmt.Fprint(w, "    ")
		printParamPretty(w, leaf)
	}
}

// rangePretty renders Min/Max for the pretty view.
func rangePretty(p iagents.CardParam) string {
	trim := func(f float64) string { return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", f), "0"), ".") }
	switch {
	case p.Min != nil && p.Max != nil:
		return trim(*p.Min) + ".." + trim(*p.Max)
	case p.Min != nil:
		return ">=" + trim(*p.Min)
	default:
		return "<=" + trim(*p.Max)
	}
}

// printCardPretty writes a compact human-readable view of the lean card:
// identity header (with per-identity preconditions), the sorted capability
// matrix, the has_parameters cue and declared skills. Remote cards carry
// agent-controlled Name/Description strings, so every such field is
// ANSI-stripped before hitting the terminal. Nil cards degrade to a
// placeholder line rather than panicking.
func printCardPretty(w io.Writer, card *iagents.AgentCard) {
	if card == nil {
		fmt.Fprintln(w, "(no card)")
		return
	}
	// Dynamic cards carry a Name; static cards fall back to the provider label.
	name := card.Name
	if name == "" {
		name = card.ProviderLabel
	}
	fmt.Fprintf(w, "%s (%s)\n", stripANSI(name), card.AgentID)
	if card.Description != "" {
		fmt.Fprintf(w, "  %s\n", stripANSI(card.Description))
	}
	if len(card.Identity) > 0 {
		ids := make([]string, 0, len(card.Identity))
		for _, spec := range card.Identity {
			id := string(spec.Type)
			if spec.Precondition != "" {
				id += " (precondition: " + stripANSI(spec.Precondition) + ")"
			}
			ids = append(ids, id)
		}
		fmt.Fprintf(w, "  identity: %s\n", strings.Join(ids, ", "))
	}

	fmt.Fprintln(w, "  capabilities:")
	// Capabilities is a closed struct; iterate in fixed alphabetical key order.
	keys := []string{
		iagents.CapArtifactDownload,
		iagents.CapContextDelete,
		iagents.CapContextGet,
		iagents.CapContextList,
		iagents.CapFileInput,
		iagents.CapInputRequired,
		iagents.CapTaskCancel,
		iagents.CapTaskGet,
		iagents.CapTaskList,
	}
	sort.Strings(keys)
	for _, k := range keys {
		mark := "no"
		if card.Supports(k) {
			mark = "yes"
		}
		fmt.Fprintf(w, "    %-20s %s\n", k, mark)
	}

	if len(card.HasParameters) > 0 {
		fmt.Fprintf(w, "  parameters: %s\n", strings.Join(card.HasParameters, ", "))
		fmt.Fprintf(w, "    (use --operation <verb> for details, e.g. lark-cli agents card %s --operation %s)\n",
			safeRefOrPlaceholder(card), card.HasParameters[0])
	}
	if card.ParametersSource != "" {
		fmt.Fprintf(w, "  parameters_source: %s (template-level declaration; the platform is authoritative per agent)\n", card.ParametersSource)
	}

	if len(card.Skills) > 0 {
		fmt.Fprintln(w, "  skills:")
		for _, sk := range card.Skills {
			name := sk.Name
			if name == "" {
				name = sk.ID
			}
			fmt.Fprintf(w, "    %s\n", stripANSI(name))
		}
	}
}

// safeRefOrPlaceholder reconstructs the card's ref for the pretty hint when it
// passes the interpolation whitelist, else a placeholder.
func safeRefOrPlaceholder(card *iagents.AgentCard) string {
	ref := card.Provider + ":" + card.AgentID
	if safeNextRef(ref) {
		return ref
	}
	return "<agent_ref>"
}
