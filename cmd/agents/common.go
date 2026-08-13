// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package agent implements the `agent` command tree: a provider-agnostic
// surface over remote A2A agents. This file holds the shared
// command-layer helpers: ref→provider resolution, --param validation against a
// Card, success-envelope emission, capability gating, and wait/watch polling.
package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// supportedIdentities is the identity whitelist enforced for every agent
// command; provider cards advertise (a subset of) the same set.
var supportedIdentities = []string{string(core.AsUser), string(core.AsBot)}

// sleep is the package-level, test-injectable backoff sleep. It blocks for d or
// until ctx is done, returning true if the full duration elapsed and false if
// ctx was canceled first. Tests swap it for a no-op.
var sleep = func(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// resolveSpec is the fully-offline resolution path: it resolves the effective
// identity, enforces both the global user|bot whitelist and the provider's
// advertised identity subset, and looks up the AgentSpec
// addressed by ref — WITHOUT constructing a client or touching the network. It
// is the FIRST step of every verb, so a malformed ref, an unknown scheme /
// unknown catalog id, AND a capability gate all surface at exit 2 BEFORE the
// config gate — an unconfigured user still gets the precise error, not
// not_configured. A real API verb then calls runtimeFor to build the client.
func resolveSpec(f *cmdutil.Factory, cmd *cobra.Command, ref, asStr string) (iagents.Provider, *iagents.AgentSpec, string, core.Identity, error) {
	prov, spec, agentID, id, err := resolveSpecForCard(f, cmd, ref, asStr)
	if err != nil {
		return iagents.Provider{}, nil, "", "", err
	}
	if err := checkProviderIdentity(f, id, prov); err != nil {
		return iagents.Provider{}, nil, "", "", err
	}
	return prov, spec, agentID, id, nil
}

// resolveSpecForCard resolves the effective identity and ref without enforcing
// the provider's identity subset. A static card is the discovery surface that
// tells callers which identities the provider supports, so it must remain
// available even when the current/default identity is unsupported. Actual
// operations use resolveSpec above and therefore still enforce the subset.
func resolveSpecForCard(f *cmdutil.Factory, cmd *cobra.Command, ref, asStr string) (iagents.Provider, *iagents.AgentSpec, string, core.Identity, error) {
	id := f.ResolveAs(cmd.Context(), cmd, core.Identity(asStr))
	if err := f.CheckIdentity(id, supportedIdentities); err != nil {
		return iagents.Provider{}, nil, "", "", err
	}
	prov, spec, agentID, err := iagents.LookupSpec(ref)
	if err != nil {
		// ParseRef / unknown-scheme / unknown-id errors carry the validation
		// wording; promote them to a typed validation error (with a recovery hint)
		// so RunE never returns a bare error and the exit code / subtype are stable.
		return iagents.Provider{}, nil, "", "", wrapRefResolveError(err)
	}
	return prov, spec, agentID, id, nil
}

func providerSupportsIdentity(prov iagents.Provider, id core.Identity) bool {
	for _, identity := range prov.Identities {
		if string(identity.Type) == string(id) {
			return true
		}
	}
	return false
}

func checkProviderIdentity(f *cmdutil.Factory, id core.Identity, prov iagents.Provider) error {
	providerIdentities := make([]string, 0, len(prov.Identities))
	for _, identity := range prov.Identities {
		providerIdentities = append(providerIdentities, string(identity.Type))
	}
	return f.CheckIdentity(id, providerIdentities)
}

// runtimeFor builds the identity-pinned Runtime for a verb that actually calls
// the remote API. It requires a configured client (not_configured / exit 3 here
// is correct for a real API call). agentID is the resolved agent this call
// addresses (from the ref), exposed to hooks via rt.AgentID(); params is the
// validated business-parameter map (defaults backfilled) exposed via
// rt.Params() — pass nil on paths that carry no business params (card's
// Describe enrichment).
func runtimeFor(f *cmdutil.Factory, id core.Identity, agentID string, params map[string]string) (iagents.Runtime, error) {
	apiClient, err := f.NewAPIClient()
	if err != nil {
		return nil, err
	}
	return &cmdRuntime{client: apiClient, as: id, agentID: agentID, params: params}, nil
}

// wrapRefResolveError promotes a ParseRef / provider-resolution error to a
// validation typed error (subtype invalid_argument, exit 2) and attaches the
// recovery hint keyed to the failure mode: a malformed ref (no ':' / empty
// half — matched via the ErrInvalidRef sentinel) teaches the <scheme>:<agent_id>
// shape; an unknown scheme points at `agents list` to discover the available
// providers. Both hints are copy-pasteable next steps, not just wording.
func wrapRefResolveError(err error) error {
	// LookupSpec's unknown-catalog-id case is ALREADY a typed validation error
	// carrying a scheme-scoped hint (`agents list <scheme>`); pass it through
	// instead of flattening it via err.Error() and overwriting that hint with the
	// generic provider-list one. Only the untyped ParseRef sentinel / unknown-
	// scheme errors need wrapping.
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	e := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err.Error()).WithCause(err)
	if errors.Is(err, iagents.ErrInvalidRef) {
		return e.WithHint("agent_ref looks like <scheme>:<agent_id>, e.g. base:assistant")
	}
	return e.WithHint("run lark-cli agents list to see the available providers")
}

// cardHint builds the "check the agent card" hint. The ref is user-echoed
// input: when it passes the safeNextRef whitelist the hint carries the
// copy-pasteable command; otherwise it degrades to plain guidance without any
// interpolated command (a ref containing spaces would make the command
// non-copy-pasteable, and the hint is what an AI copies verbatim).
func cardHint(ref, what string) string {
	if safeNextRef(ref) {
		return fmt.Sprintf("run lark-cli agents card %s to see %s", ref, what)
	}
	return fmt.Sprintf("read this agent's capability card (agents card) to confirm %s", what)
}

// emitTask writes a task result: the standard success envelope carrying
// meta.next[] hints for AI callers, or — with format=pretty and no --jq —
// the key:value human view. Because the agent's messages/artifacts are
// untrusted external content, the payload is run through content-safety
// scanning before emission on BOTH paths (and the pretty path additionally
// ANSI-strips agent text). A --jq expression, when the leaf command registers
// one, implies structured JSON and filters stdout.
func emitTask(f *cmdutil.Factory, cmd *cobra.Command, task *iagents.AgentTask, next []output.NextAction, format string, notices ...string) error {
	out := f.IOStreams.Out
	errOut := f.IOStreams.ErrOut

	scan := output.ScanForSafety(cmd.CommandPath(), task, errOut)
	if scan.Blocked {
		return scan.BlockErr
	}

	// Normalization notices (provider contract defects) must be visible on
	// BOTH surfaces: stderr for humans, envelope _notice for the JSON consumer.
	var defect string
	for _, n := range notices {
		if n != "" {
			defect = n
			fmt.Fprintf(errOut, "notice: %s\n", n)
		}
	}

	if format == "pretty" && jqExpr(cmd) == "" {
		if scan.Alert != nil {
			output.WriteAlertWarning(errOut, scan.Alert)
		}
		printTaskPretty(out, task)
		return nil
	}

	env := output.Envelope{
		OK:       true,
		Identity: string(f.ResolvedIdentity),
		Data:     task,
		Notice:   output.GetNotice(),
	}
	if defect != "" {
		if env.Notice == nil {
			env.Notice = map[string]interface{}{}
		}
		env.Notice["provider_defect"] = defect
	}
	if len(next) > 0 {
		// Identity carry follows the CLI-family convention: only an EXPLICIT --as is
		// pinned into the suggestion, because an explicit non-default identity would
		// otherwise fall back to the default on verbatim replay and read another
		// principal's task store. An implicit identity stays unpinned — it
		// re-resolves to the same answer in the same environment.
		carryAsIntoNext(cmd, f, next)
		env.Meta = &output.Meta{Next: next}
	}
	if scan.Alert != nil {
		env.ContentSafetyAlert = scan.Alert
	}

	if jq := jqExpr(cmd); jq != "" {
		if scan.Alert != nil {
			output.WriteAlertWarning(errOut, scan.Alert)
		}
		return output.JqFilter(out, env, jq)
	}
	output.PrintJson(out, env)
	return nil
}

// scanAndEmitData is the shared scan-then-emit path for the read leaves whose
// payload now carries untrusted agent-authored text — task list
// (TaskSummary.Summary), context list, and context get
// (ContextDetail.ActiveTask.Summary). These used to PrintJson directly and so
// BYPASSED content-safety; like emitTask they now run output.ScanForSafety on
// the payload BEFORE emission on every path: a block returns the typed block
// error, a warn attaches the alert to the JSON envelope (and prints a stderr
// warning on the pretty / jq paths). data is the Envelope.Data payload (and what
// is scanned); meta is an optional *output.Meta (list count, nil for a single
// detail); pretty renders the --format pretty human view and is skipped when a
// --jq expression forces structured JSON.
func scanAndEmitData(f *cmdutil.Factory, cmd *cobra.Command, format string, data any, meta *output.Meta, pretty func(io.Writer)) error {
	out := f.IOStreams.Out
	errOut := f.IOStreams.ErrOut

	scan := output.ScanForSafety(cmd.CommandPath(), data, errOut)
	if scan.Blocked {
		return scan.BlockErr
	}

	if format == "pretty" && jqExpr(cmd) == "" {
		if scan.Alert != nil {
			output.WriteAlertWarning(errOut, scan.Alert)
		}
		pretty(out)
		return nil
	}

	env := output.Envelope{
		OK:       true,
		Identity: string(f.ResolvedIdentity),
		Data:     data,
		Meta:     meta,
		Notice:   output.GetNotice(),
	}
	if scan.Alert != nil {
		env.ContentSafetyAlert = scan.Alert
	}
	if jq := jqExpr(cmd); jq != "" {
		if scan.Alert != nil {
			output.WriteAlertWarning(errOut, scan.Alert)
		}
		return output.JqFilter(out, env, jq)
	}
	output.PrintJson(out, env)
	return nil
}

// jqExpr reads the --jq flag value if the leaf command registered one; absent
// otherwise.
func jqExpr(cmd *cobra.Command) string {
	if cmd == nil { // options structs built directly in tests may carry no Cmd
		return ""
	}
	if f := cmd.Flags().Lookup("jq"); f != nil {
		return f.Value.String()
	}
	return ""
}

// resolvedBrand returns the brand the agent commands filter/gate against: the
// logged-in account's Config().Brand when Config resolves and is non-empty,
// else BrandFeishu (the offline/unconfigured default, consistent with
// core.ParseBrand mapping unknown→feishu). It is nil-safe (a nil Factory or a
// nil Config hook yields feishu), so the offline gates hold before config init.
func resolvedBrand(f *cmdutil.Factory) core.LarkBrand {
	if f == nil || f.Config == nil {
		return core.BrandFeishu
	}
	cfg, err := f.Config()
	if err != nil || cfg == nil || cfg.Brand == "" {
		return core.BrandFeishu
	}
	return cfg.Brand
}

// unavailableForBrandError returns the unavailable_for_brand validation error
// (exit 2) — the brand sibling of capabilityError. `what` is the human-facing
// capability name (e.g. "task cancel"); an empty `what` is the whole-agent case
// ("agent '<ref>' is not available under <brand>"). The hint points at the card
// for the current brand (cardHint interpolates ref only when it is whitelisted).
func unavailableForBrandError(ref, what string, brand core.LarkBrand) error {
	var msg string
	if what == "" {
		msg = fmt.Sprintf("agent '%s' is unavailable under the %s brand", ref, brand)
	} else {
		msg = fmt.Sprintf("agent '%s' does not offer '%s' under the %s brand", ref, what, brand)
	}
	return errs.NewValidationError(errs.SubtypeUnavailableForBrand, "%s", msg).
		WithHint("%s", cardHint(ref, "the capabilities available under the current brand"))
}

// brandGate is the whole-agent brand visibility gate: a spec whose declared
// Brands exclude the resolved brand returns unavailable_for_brand (exit 2,
// offline) before any network call. Placed right after the capability/offline
// gates in every verb path.
func brandGate(f *cmdutil.Factory, spec *iagents.AgentSpec, ref string) error {
	if brand := resolvedBrand(f); !iagents.SpecAvailableForBrand(spec, brand) {
		return unavailableForBrandError(ref, "", brand)
	}
	return nil
}

// opBrandGate is the per-capability brand gate: a WIRED op whose declared Brands
// exclude the resolved brand returns unavailable_for_brand (exit 2, offline).
// `what` is the human capability name. It assumes the whole-agent gate (brandGate)
// already passed. Core ops (Send/GetTask) normally declare no Brands, so this is
// a no-op for them unless a provider scopes them explicitly.
func opBrandGate(f *cmdutil.Factory, brands []core.LarkBrand, ref, what string) error {
	if brand := resolvedBrand(f); !iagents.OpAvailableForBrand(brands, brand) {
		return unavailableForBrandError(ref, what, brand)
	}
	return nil
}

// capabilityError returns the unsupported_capability validation error (exit 2)
// used for capability gating: capHuman is the human-facing action (e.g.
// "task cancel"), capKey the Card capability key (e.g. task_cancel). The hint
// interpolates ref only when it passes the whitelist (cardHint).
func capabilityError(ref, capHuman, capKey string) error {
	return errs.NewValidationError(
		errs.SubtypeUnsupportedCapability,
		"agent '%s' does not support '%s' (capability %s=false)", ref, capHuman, capKey,
	).WithHint("%s", cardHint(ref, "the supported capabilities"))
}

// normalizeTask canonicalizes a provider task the moment it enters the command
// layer: IsTerminal is re-derived from State (the single source of truth, so a
// provider that mis-fills the flag can never skew watch exit codes or an AI
// caller's stop-polling decision), and the input_required question group runs
// the central normalization (size caps, empty options → absent, bare
// prompt → one ordinary free-text question, non-conforming keys → whole-group
// degrade). The returned notice — a provider defect worth seeing — must reach
// the caller's output surface (emitTask routes it into the JSON envelope
// _notice and onto stderr for pretty) instead of being silently smoothed over.
// nil-safe.
func normalizeTask(t *iagents.AgentTask) (notice string) {
	if t == nil {
		return ""
	}
	t.IsTerminal = t.State.IsTerminal()
	return iagents.NormalizeInputRequired(t)
}

// normalizeTaskSummaries derives IsTerminal from State for every summary (same
// single-source rule as normalizeTask), returning the slice for chaining.
func normalizeTaskSummaries(ts []iagents.TaskSummary) []iagents.TaskSummary {
	for i := range ts {
		ts[i].IsTerminal = ts[i].State.IsTerminal()
	}
	return ts
}

// pollToStop polls getTask with exponential backoff (1s → 5s cap) until the
// task hits a stop condition (terminal, input_required, or auth_required)
// or ctx is done. A timeout is not a failure: it returns the most recent
// task with a nil error, letting the caller print the current state (exit 0). A
// provider GetTask error is surfaced. getTask is a bound closure over the
// resolved spec + runtime (spec.GetTask(ctx, rt, id)), so pollToStop stays
// provider-neutral and testable.
func pollToStop(ctx context.Context, getTask func(context.Context, string) (*iagents.AgentTask, error), taskID string) (*iagents.AgentTask, error) {
	const (
		initialDelay = time.Second
		maxDelay     = 5 * time.Second
	)
	var last *iagents.AgentTask
	delay := initialDelay
	for {
		task, err := getTask(ctx, taskID)
		if err != nil {
			return last, err
		}
		last = task
		if task.State.ShouldStopPolling() {
			return task, nil
		}
		if ctx.Err() != nil {
			return last, nil //nolint:nilerr // a poll timeout is an observation-window close, not a task failure — return the last task with exit 0
		}
		if !sleep(ctx, delay) {
			// ctx canceled during backoff → observation window closed, not a
			// task failure.
			return last, nil
		}
		if delay < maxDelay {
			if delay *= 2; delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// semanticExitError maps a wait/watch terminal task to the semantic exit code:
// a non-successful terminal state (failed/rejected/canceled) yields a
// silent exit-1 signal; any other state (including a successful terminal or a
// non-terminal stop like input_required) yields nil. A nil task yields nil.
func semanticExitError(task *iagents.AgentTask) error {
	if task == nil || !task.IsTerminal {
		return nil
	}
	switch task.State {
	case iagents.StateFailed, iagents.StateRejected, iagents.StateCanceled:
		return output.ErrBare(1)
	default:
		return nil
	}
}

// listMeta builds the list-class meta: count for a non-empty list, nil (no
// meta at all) for an empty one. Count is omitempty at the shared envelope
// level, so an empty list would otherwise degrade to the ambiguous "meta": {}
// third shape; absent-with-documented-rule beats an empty object. (Emitting an
// explicit "count": 0 would need the shared Meta.Count to become a pointer —
// a repo-wide change deliberately out of this package's blast radius.)
func listMeta(n int) *output.Meta {
	if n == 0 {
		return nil
	}
	return &output.Meta{Count: n}
}

// Pagination flag defaults / bounds, shared by the three paginated list leaves
// (task list, context list, list <scheme>).
const (
	defaultPageSize = 20
	minPageSize     = 1
	maxPageSize     = 100
)

// addPageFlags registers the shared --page-size / --page-token flags on a
// paginated list leaf. Size defaults to defaultPageSize (a bare list returns the
// first page); an empty token asks for the first page.
func addPageFlags(cmd *cobra.Command, pageSize *int, pageToken *string) {
	cmd.Flags().IntVar(pageSize, "page-size", defaultPageSize, "page size (1-100)")
	cmd.Flags().StringVar(pageToken, "page-token", "", "page_token returned by the previous page; empty fetches the first page")
}

// validatePageSize enforces the [minPageSize,maxPageSize] range as a client-side
// invalid_argument validation error (exit 2) before any provider is built, so a
// nonsense size never reaches the network and holds under a nil Factory.
func validatePageSize(n int) error {
	if n < minPageSize || n > maxPageSize {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--page-size must be between %d and %d, got %d", minPageSize, maxPageSize, n).
			WithParam("--page-size").
			WithHint("resend with a page size between %d and %d", minPageSize, maxPageSize)
	}
	return nil
}

// listMetaPage builds the page-aware list meta: the shared pagination block plus
// the next-page action(s). It preserves listMeta's "no empty {}" rule — nil is
// returned ONLY when the page is empty AND there is no next page AND there is no
// next action, so an otherwise-absent meta never degrades to the ambiguous
// "meta": {} shape.
//
// The agents verbs are a single-page cursor API: one command call fetches exactly
// one page and hands the caller a cursor, rather than walking pages internally
// like the --page-all shortcuts. Mapping onto the shared PaginationMeta therefore
// fixes Pages at 1 (one page consumed per call) and reads Complete as "this page
// is the last one". Count is left unset so Items is the single source for the
// record count.
func listMetaPage(count int, info iagents.PageInfo, next []output.NextAction) *output.Meta {
	if count == 0 && !info.HasMore && len(next) == 0 {
		return nil
	}
	return &output.Meta{
		Pagination: &output.PaginationMeta{
			Complete:  !info.HasMore,
			Pages:     1,
			Items:     count,
			NextToken: info.NextToken,
		},
		Next: next,
	}
}

// carryAsIntoNext mirrors emitTask's identity-carry rule for the paginated list
// leaves (which build their own next-actions instead of going through emitTask):
// only when the caller EXPLICITLY passed --as does the suggested next-page
// command carry the resolved identity, so an explicit non-default identity is not
// silently dropped on verbatim replay while an implicit (default/auto) identity
// stays unpinned. No-op on a nil cmd or an unchanged --as.
func carryAsIntoNext(cmd *cobra.Command, f *cmdutil.Factory, next []output.NextAction) {
	if cmd == nil || !cmd.Flags().Changed("as") {
		return
	}
	id := string(f.ResolvedIdentity)
	if id == "" {
		return
	}
	for i := range next {
		if strings.HasPrefix(next[i].Command, "lark-cli agents ") {
			next[i].Command += " --as " + id
		}
	}
}

// nextPageAction builds the single "next page" next-action for a paginated list when
// a next page exists. base is the fully-formed command up to (but not including)
// the pagination flags, e.g. "lark-cli agents task list base:assistant"; the caller
// is responsible for whitelisting the ref / scheme / context-id interpolated into
// base. The cursor is server-controlled and interpolated verbatim into a command
// the AI runs, so it must pass the safeNextID whitelist first — a failing cursor
// drops the command (the cursor still rides meta.pagination.next_token as data, so
// the caller can page manually). Returns nil when there is no next page.
func nextPageAction(base string, size int, info iagents.PageInfo) []output.NextAction {
	if !info.HasMore || info.NextToken == "" || !safeNextID(info.NextToken) {
		return nil
	}
	return []output.NextAction{{
		Label:   "next page",
		Command: fmt.Sprintf("%s --page-size %d --page-token %s", base, size, info.NextToken),
	}}
}
