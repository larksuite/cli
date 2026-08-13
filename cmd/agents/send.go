// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

// sendOptions holds all inputs for `agents send <ref>`.
type sendOptions struct {
	Factory   *cmdutil.Factory
	Cmd       *cobra.Command
	Ref       string
	Text      string
	Files     []string
	Params    []string
	ContextID string
	TaskID    string
	Answers   []string // raw --answer key=value entries, argv order
	DryRun    bool
	Yes       bool
	As        string
	Format    string
}

// NewCmdAgentSend builds `agents send <agent_ref>`: send a message to a remote
// agent, starting a new task or continuing an existing one. `--dry-run`
// validates the inputs against the agent Card and prints the request preview
// without any API call (always available). A send fires and returns the
// current task immediately; poll progress with
// `agents task get <agent_ref> <task-id> --watch` (surfaced via meta.next).
// `--file` uploads local files to the remote agent — the content leaves this
// machine. Risk=write. runF, when non-nil, replaces the production run path
// (test seam).
func NewCmdAgentSend(f *cmdutil.Factory, runF func(*sendOptions) error) *cobra.Command {
	opts := &sendOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "send <agent_ref>",
		Short: "Send a message to a remote agent (start a new task or continue an existing one)",
		Long: "Send one message to the remote agent addressed by agent_ref. Without --context-id/--task-id it starts a new task; " +
			"with --context-id (optionally --task-id) it continues the same multi-turn context; with --answer it answers the task's pending input_required question group. " +
			"--dry-run only validates locally and prints the request preview without calling the API. A send fires and returns the current task immediately; " +
			"poll progress with agents task get <agent_ref> <task-id> --watch (see meta.next).",
		Args: exactArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFormat(opts.Format); err != nil {
				return err
			}
			opts.Cmd = cmd
			opts.Ref = args[0]
			if runF != nil {
				return runF(opts)
			}
			return agentSendRun(opts)
		},
	}
	cmd.Flags().StringVar(&opts.Text, "text", "", "free-text part of the message: the body when starting or continuing a task, or an overall remark alongside --answer (--text is never one question's answer)")
	cmd.Flags().StringArrayVar(&opts.Files, "file", nil, "local file to send with the message, repeatable; the file is uploaded to the remote provider (content leaves this machine)")
	addParamFlag(cmd, &opts.Params)
	cmd.Flags().StringVar(&opts.ContextID, "context-id", "", "multi-turn context id (continue the same conversation)")
	cmd.Flags().StringVar(&opts.TaskID, "task-id", "", "continue an existing task (requires --context-id)")
	cmd.Flags().StringArrayVar(&opts.Answers, "answer", nil, "answer the pending input_required question group, repeatable: <question_id>=<option_id> for a choice question (repeat the key for multi-select), <question_id>.text=<text> for free text; requires --context-id/--task-id")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "validate locally and print the request preview without calling the API")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "confirm uploading the files named by --file to the remote agent (without it: exit 10, nothing uploaded)")
	cmd.Flags().StringVar(&opts.Format, "format", "json", formatFlagHelp)
	cmd.Flags().String("jq", "", "filter the JSON output with a jq expression")
	if f != nil {
		cmdutil.AddAPIIdentityFlag(cmd.Context(), cmd, f, &opts.As)
	} else {
		// f is nil only in construction-time unit tests; register a bare --as so
		// the flag surface is still assertable without a Factory.
		cmd.Flags().StringVar(&opts.As, "as", "", "identity type: user | bot (the identities an agent actually supports are listed by agents card)")
	}
	cmdutil.SetRisk(cmd, cmdutil.RiskWrite)
	return cmd
}

// sendMode is send's semantic mode, derived from the flags by a fixed priority
// (the user never passes a mode). The discriminator formalizes what the guards
// enforce: answer needs the pending group's task+context and takes --answer
// entries (with --text as an optional message-level remark); continue/start
// need --text.
type sendMode string

const (
	modeStart    sendMode = "start"    // no context/task/answers — a fresh task
	modeContinue sendMode = "continue" // has context (optionally task) — same conversation
	modeAnswer   sendMode = "answer"   // has --answer entries — input_required group reply
)

// answerKeyPattern is the offline --answer key grammar: a KeyPattern-conforming
// question id plus at most one case-sensitive ".text" suffix. Anything else —
// ".txt", ".TEXT", a bare ".text", two dots, a '-'-leading flag-lookalike — is
// rejected before any network access, because the only two legal key shapes are
// <qid> and <qid>.text and a near-miss silently becoming an unknown question_id
// at the provider would send the AI down the wrong recovery branch.
var answerKeyPattern = regexp.MustCompile(`^` + iagents.KeyCharsetRE + `(\.text)?$`)

// parseAnswers parses the raw --answer key=value entries into the map encoding
// (values in argv order), running every offline guard in one
// collect-all pass so a multi-error submission is fixed in one round-trip:
// key=value shape, key grammar, non-empty value, no duplicate .text entry per
// question. Exact duplicate bare values are deduplicated (an AI retry glitch is
// idempotent, not an error). Semantic validation (does the qid exist, is the
// value a legal option) is deliberately NOT here — the CLI is stateless and
// does not hold the question group; that is the provider's policy.
func parseAnswers(raw []string) (map[string][]string, error) {
	answers := make(map[string][]string, len(raw))
	var viols []string
	for _, entry := range raw {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			viols = append(viols, fmt.Sprintf("%s (not in key=value form)", entry))
			continue
		}
		if !answerKeyPattern.MatchString(key) {
			viols = append(viols, fmt.Sprintf("%s (invalid key: the only legal forms are <question_id> and <question_id>.text)", key))
			continue
		}
		if value == "" {
			viols = append(viols, fmt.Sprintf("%s (an empty answer means nothing: give an option_id for a choice question, non-empty text otherwise; omit the key for a question you do not want to answer)", key))
			continue
		}
		if _, isText := iagents.SplitAnswerKey(key); isText && len(answers[key]) > 0 {
			viols = append(viols, fmt.Sprintf("%s (.text may appear only once per question; text does not accumulate)", key))
			continue
		}
		dup := false
		for _, v := range answers[key] {
			if v == value {
				dup = true // exact duplicate → dedupe silently
				break
			}
		}
		if !dup {
			answers[key] = append(answers[key], value)
		}
	}
	if len(viols) > 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid --answer: %s", strings.Join(viols, "; ")).
			WithParam("--answer").
			WithHint("use --answer <question_id>=<option_id> for a choice question (repeat the key for multi-select) and --answer <question_id>.text=<text> for free text; fix each entry and resubmit the whole group")
	}
	return answers, nil
}

// deriveSendMode classifies the send and runs the per-mode client-side guards
// (all offline, all holding under a nil Factory). Conflicting combinations
// never silently fall back to another mode. Guard PRECEDENCE is deliberate
// mode-first: with several simultaneous mistakes the mode-defining flag's guard
// wins (e.g. --answer without --context-id reports the answer guard, not the
// missing --text) — the caller learns which MODE it got wrong before which
// field it forgot. Returns the parsed answers map for the answer mode (nil
// otherwise).
func deriveSendMode(opts *sendOptions) (sendMode, map[string][]string, error) {
	hasText := strings.TrimSpace(opts.Text) != ""
	if len(opts.Answers) > 0 {
		// answer: continues the pending group's own task, so both ids are required.
		if opts.ContextID == "" || opts.TaskID == "" {
			return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"answering a question group requires both --context-id and --task-id").
				WithParam("--answer").
				WithHint("--answer must come with the --context-id/--task-id of the task holding the group (copy the command template from meta.next of task get)")
		}
		answers, err := parseAnswers(opts.Answers)
		if err != nil {
			return "", nil, err
		}
		// --text stays optional here: it is the message-level remark, never a
		// question's answer.
		return modeAnswer, answers, nil
	}
	if opts.TaskID != "" && opts.ContextID == "" {
		return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--task-id must be used together with --context-id").
			WithParam("--task-id").
			WithHint("add --context-id <ctx-id> and resend; the task's context_id is in the output of lark-cli agents task get <agent_ref> <task-id>")
	}
	if !hasText {
		return "", nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--text must contain non-whitespace characters").
			WithParam("--text").
			WithHint(`add --text "<message>" and resend; to answer a question group use --answer <question_id>=<option_id> or --answer <question_id>.text=<text>`)
	}
	if opts.ContextID != "" {
		return modeContinue, nil, nil
	}
	return modeStart, nil, nil
}

// agentSendRun validates the send inputs, resolves the provider, and either
// prints a dry-run preview or dispatches the message. The mode guards run
// first so they never touch the network and hold even under a nil Factory. A
// send fires once and returns the current task immediately (exit 0); the
// caller polls progress via the meta.next `task get ... --watch` hint.
func agentSendRun(opts *sendOptions) error {
	_, answers, err := deriveSendMode(opts)
	if err != nil {
		return err
	}
	if err := validateSendFiles(opts.Files); err != nil {
		return err
	}

	f := opts.Factory
	// Resolution + --param validation + --dry-run are fully offline, so they work
	// (and surface validation as exit 2) before the config gate. The card is
	// built with rt=nil (capability matrix only) for the file gate; --param
	// validation reads the send operation's own declaration.
	prov, spec, agentID, id, err := resolveSpec(f, opts.Cmd, opts.Ref, opts.As)
	if err != nil {
		return err
	}
	// Whole-agent brand gate (offline), before the card / any network access.
	if err := brandGate(f, spec, opts.Ref); err != nil {
		return err
	}
	// Send is a core op; gate it on its own Brands too (normally empty ⇒ no-op).
	if err := opBrandGate(f, spec.Send.Brands, opts.Ref, "send"); err != nil {
		return err
	}
	card := iagents.BuildCard(opts.Cmd.Context(), prov, spec, agentID, resolvedBrand(f), nil)
	vp, err := validateParams(opts.Params, spec.Send.Params, iagents.VerbSend, spec, opts.Ref)
	if err != nil {
		return err
	}

	in := iagents.SendInput{
		Text:      opts.Text,
		Files:     opts.Files,
		ContextID: opts.ContextID,
		TaskID:    opts.TaskID,
		Answers:   answers,
	}

	// Capability gates run BEFORE the dry-run branch so a preview is never a
	// capability bypass: an agent that declares input_required=false / file_input=false
	// rejects --answer / --file with unsupported_capability whether or not --dry-run
	// is set — the gate is a local Card check, no network, so the verdict must match
	// a real send. (The CONFIRMATION gate for --file stays AFTER dry-run below, since
	// dry-run uploads nothing and is exempt from --yes.)
	if len(in.Answers) > 0 && !card.Supports(iagents.CapInputRequired) {
		return capabilityError(opts.Ref, "send with --answer", iagents.CapInputRequired)
	}
	if len(in.Files) > 0 && !card.Supports(iagents.CapFileInput) {
		return capabilityError(opts.Ref, "send with --file", iagents.CapFileInput)
	}

	// --dry-run is a client-side behavior: always available, never gated by a
	// dry_run capability, and never touches the API. The capability gates above
	// have already run; only the confirmation gate below is dry-run-exempt.
	if opts.DryRun {
		return emitDryRun(f, opts.Cmd, opts.Ref, in, vp.Resolved, opts.Format)
	}

	// --file exfiltrates local file content off this machine (the provider reads
	// the file and uploads it to the remote agent). That is an irreversible,
	// CLI-enforced high-risk write: a real send that would upload requires --yes,
	// returning confirmation_required (exit 10) before any network access. dry-run
	// above is exempt — it never uploads.
	if len(in.Files) > 0 && !opts.Yes {
		return errs.NewConfirmationRequiredError(errs.RiskHighRiskWrite, "agents send --file",
			"--file uploads local file content to the remote agent (it leaves this machine and cannot be recalled)").
			WithHint("add --yes to confirm sending these files")
	}

	// A real send calls the API, so it needs a configured client; build the
	// identity-pinned runtime now (not_configured / exit 3 here is correct).
	rt, err := runtimeFor(f, id, agentID, vp.Resolved)
	if err != nil {
		return err
	}

	// Local scope preflight: after runtimeFor, before the API call. The check is
	// all-or-nothing — any real API verb requires the full scope set the
	// provider declares for the resolved identity.
	if err := preflightScopesForRef(f, id, opts.Ref); err != nil {
		return err
	}

	task, err := spec.Send.Handler(opts.Cmd.Context(), rt, in)
	if err != nil {
		return err
	}
	notice := normalizeTask(task)

	// A send fires and returns the current task immediately (exit 0). Progress is
	// polled separately via the meta.next `task get <agent_ref> <task-id> --watch`
	// hint — send no longer blocks on the task reaching a stop condition.
	return emitTask(f, opts.Cmd, task, nextForTask(opts.Ref, task, spec, vp.Given, iagents.VerbSend), opts.Format, notice)
}

// validateSendFiles is the local gate on --file paths, running before any
// capability/confirmation gate or network access (dry-run included): every
// path must be a relative-within-CWD (the lark-shared safety rule the docs
// promise) EXISTING regular file. Violations are collected and reported in one
// pass, mirroring the --param collect-all style, so a multi-file send is fixed
// in one round-trip. Without this gate a bad path used to be discovered only
// by the provider (or worse, silently "uploaded").
func validateSendFiles(files []string) error {
	var viols []string
	for _, p := range files {
		abs, err := validate.SafeInputPath(p)
		if err != nil {
			viols = append(viols, fmt.Sprintf("%s (only relative paths inside the CWD are accepted)", p))
			continue
		}
		st, err := os.Stat(abs)
		switch {
		case err != nil:
			viols = append(viols, fmt.Sprintf("%s (does not exist or is not readable)", p))
		case st.IsDir():
			viols = append(viols, fmt.Sprintf("%s (is a directory; --file accepts files only)", p))
		}
	}
	if len(viols) == 0 {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"invalid --file path: %s", strings.Join(viols, "; ")).
		WithParam("--file").
		WithHint("--file accepts only existing files at relative paths inside the current directory; fix each one and resend")
}

// emitDryRun writes the dry-run preview: {dry_run:true, would_send:{…}}
// reconstructed from the validated input, so a caller can inspect exactly what
// a real send would post without contacting the agent. format=pretty (no --jq)
// renders the same fields as key: value lines instead of the envelope.
func emitDryRun(f *cmdutil.Factory, cmd *cobra.Command, ref string, in iagents.SendInput, params map[string]string, format string) error {
	if format == "pretty" && jqExpr(cmd) == "" {
		out := f.IOStreams.Out
		fmt.Fprintln(out, "dry_run: true")
		fmt.Fprintf(out, "agent_ref: %s\n", kvValue(ref))
		fmt.Fprintf(out, "text: %s\n", truncateRunes(kvValue(in.Text), 120))
		if len(in.Files) > 0 {
			fmt.Fprintf(out, "files: %d\n", len(in.Files))
		}
		if len(params) > 0 {
			fmt.Fprintf(out, "params: %d\n", len(params))
		}
		if in.ContextID != "" {
			fmt.Fprintf(out, "context_id: %s\n", kvValue(in.ContextID))
		}
		if in.TaskID != "" {
			fmt.Fprintf(out, "task_id: %s\n", kvValue(in.TaskID))
		}
		if len(in.Answers) > 0 {
			fmt.Fprintf(out, "answers: %d\n", len(in.Answers))
		}
		return nil
	}

	would := map[string]interface{}{
		"agent_ref": ref,
		"text":      in.Text,
	}
	if len(in.Files) > 0 {
		would["files"] = in.Files
	}
	if len(params) > 0 {
		// Final values after default backfill: preview equals what is sent.
		would["params"] = params
	}
	if in.ContextID != "" {
		would["context_id"] = in.ContextID
	}
	if in.TaskID != "" {
		would["task_id"] = in.TaskID
	}
	if len(in.Answers) > 0 {
		// The key encoding is previewed verbatim: preview equals what is sent.
		would["answers"] = in.Answers
	}
	env := output.Envelope{
		OK:       true,
		Identity: string(f.ResolvedIdentity),
		Data: map[string]interface{}{
			"dry_run":    true,
			"would_send": would,
		},
		Notice: output.GetNotice(),
	}
	if jq := jqExpr(cmd); jq != "" {
		return output.JqFilter(f.IOStreams.Out, env, jq)
	}
	output.PrintJson(f.IOStreams.Out, env)
	return nil
}

// nextIDPattern is the character whitelist for server-supplied identifiers
// (task_id / context_id / question_id) before they are interpolated into a
// meta.next command: alphanumeric first character, then letters, digits, '_' and
// '-'. It is deliberately stricter than validate.ResourceName, which is a
// URL-path denylist and would pass shell metacharacters — and meta.next is
// executed verbatim, so a server-controlled id is a command-injection surface.
// The alphanumeric first character also rejects flag-lookalike ids ("--text",
// "-o") that would hijack the flag surface. It matches iagents.KeyPattern by
// construction; the two layers must agree.
var nextIDPattern = regexp.MustCompile(`^` + iagents.KeyCharsetRE + `$`)

// safeNextID reports whether s may be interpolated into a meta.next command.
func safeNextID(s string) bool {
	return nextIDPattern.MatchString(s)
}

// nextRefPattern is the whitelist for a user-supplied ref before interpolation
// into a meta.next or hint command: the safeNextID charset on both sides of one
// ':'. A ref is not server-controlled, so the threat model is copy-paste breakage
// rather than injection — a failing ref simply drops the command hint.
var nextRefPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+:[A-Za-z0-9_-]+$`)

// safeNextRef reports whether ref may be interpolated into a meta.next / hint
// command string.
func safeNextRef(ref string) bool {
	return nextRefPattern.MatchString(ref)
}

// nextForTask builds the meta.next[] hints for a task: terminal → read the
// detail / download artifacts, still running → the bounded poll, input_required
// → the answer command, auth_required → the re-authorize flow (auth login, not a
// text continuation). Callers execute these verbatim, so everything interpolated
// passes a whitelist first: a failing ref or task_id suppresses the whole hint,
// while a failing context_id degrades to the <context_id> placeholder. A command
// carrying <...> is marked Template.
//
// Business parameters for each suggested command's TARGET verb ride along per the
// three-way rule (see paramArgsFor), so a required parameter cannot fall off the
// chain. given is only what the caller explicitly provided; spec may be nil in
// construction-time tests. caller is the verb that produced this output: task get
// must not suggest the command just run, or a caller following meta.next
// verbatim would loop on itself.
func nextForTask(ref string, task *iagents.AgentTask, spec *iagents.AgentSpec, given map[string]string, caller string) []output.NextAction {
	if !safeNextRef(ref) {
		return nil
	}
	if task == nil || task.TaskID == "" || !safeNextID(task.TaskID) {
		return nil
	}
	if task.State.ShouldStopPolling() {
		if task.State == iagents.StateAuthRequired {
			// auth_required is an agent-side task state: the end user must
			// (re)authorize in the agent. It is neither a CLI scope error nor a text
			// continuation like input_required, so point at the re-authorize flow.
			// The concrete scopes are the agent's own declared set, so --scope stays a
			// placeholder → Template. ref/task_id are whitelisted above, so echoing
			// the re-check command in the label is safe; it carries task_get's
			// parameters too, since this is the one next that leaves the agent subtree
			// and must still not drop a required parameter.
			recheckArgs, _ := paramArgsFor(spec, iagents.VerbTaskGet, given)
			return []output.NextAction{{
				Label:    fmt.Sprintf("Re-check the task after re-authorizing (scopes are the agent's own; re-check: lark-cli agents task get %s %s%s)", ref, task.TaskID, recheckArgs),
				Command:  `lark-cli auth login --scope "<required_scopes>"`,
				Template: true,
			}}
		}
		if task.State == iagents.StateInputRequired {
			// A task paused on a question group expands one template per question so
			// the caller never hand-assembles the answer grammar: bare <option_id>
			// for a choice, marked repeatable for multi-select, .text for free text.
			// Every value is a placeholder, so the hint is always a template — which
			// is also why a missing or whitelist-failing context_id can degrade to
			// the <context_id> placeholder instead of dropping the hint. A
			// server-supplied question_id failing safeNextID degrades the whole hint
			// to the free-text continuation rather than emitting a key the CLI's own
			// guard would reject.
			ctxID := task.ContextID
			if ctxID == "" || !safeNextID(ctxID) {
				ctxID = "<context_id>"
			}
			sendArgs, _ := paramArgsFor(spec, iagents.VerbSend, given)
			if ir := task.InputRequired; (spec == nil || spec.InputRequired) && ir != nil && len(ir.Questions) > 0 {
				parts := make([]string, 0, len(ir.Questions))
				for _, q := range ir.Questions {
					if !safeNextID(q.QuestionID) {
						parts = nil
						break
					}
					switch {
					case len(q.Options) == 0:
						parts = append(parts, fmt.Sprintf("--answer %s.text=<text>", q.QuestionID))
					case q.MultiSelect:
						parts = append(parts, fmt.Sprintf("--answer %s=<option_id, repeatable>", q.QuestionID))
					default:
						parts = append(parts, fmt.Sprintf("--answer %s=<option_id>", q.QuestionID))
					}
				}
				if parts != nil {
					return []output.NextAction{{
						Label:    "Relay the question group to the user and submit their answers (answer on their behalf only when an earlier instruction already determines it, and state the basis); for a question where no option fits use <question_id>.text=<text>",
						Command:  fmt.Sprintf("lark-cli agents send %s --context-id %s --task-id %s %s%s", ref, ctxID, task.TaskID, strings.Join(parts, " "), sendArgs),
						Template: true,
					}}
				}
			}
			// No structured group (the provider supplied none and normalization had
			// nothing to synthesize from): plain free-text continuation, where the
			// provider treats a message to its paused task as the answer.
			return []output.NextAction{{
				Label:    "Continue the same task with the additional input",
				Command:  fmt.Sprintf("lark-cli agents send %s --context-id %s --task-id %s --text <your_reply>%s", ref, ctxID, task.TaskID, sendArgs),
				Template: true,
			}}
		}
		// Terminal: suggest reading the final detail, plus a ready-made download
		// command per artifact (so the AI never has to hand-craft the
		// `task get --artifact` form itself; -o stays a placeholder → template).
		// When the caller IS task get, the detail suggestion would be a self-loop
		// (the exact command just executed) — drop it and keep only the artifact
		// increments.
		var next []output.NextAction
		if caller != iagents.VerbTaskGet {
			getArgs, getTpl := paramArgsFor(spec, iagents.VerbTaskGet, given)
			next = append(next, output.NextAction{
				Label:    "View the task detail and artifacts",
				Command:  fmt.Sprintf("lark-cli agents task get %s %s%s", ref, task.TaskID, getArgs),
				Template: getTpl,
			})
		}
		next = append(next, artifactNext(ref, task, spec, given)...)
		return next
	}
	getArgs, getTpl := paramArgsFor(spec, iagents.VerbTaskGet, given)
	return []output.NextAction{{
		Label:    "Poll until a stop condition (bounded; if it is still running at expiry, watch again the same way)",
		Command:  fmt.Sprintf("lark-cli agents task get %s %s --watch --timeout %ds%s", ref, task.TaskID, int(defaultWatchTimeout.Seconds()), getArgs),
		Template: getTpl,
	}}
}

// artifactNext builds one ready-made download command per artifact of a
// terminal task: only when the spec wires DownloadArtifact, only for artifact
// ids that pass the whitelist (a failing id skips just that artifact), always
// template (the -o save path is the caller's choice). Params carry per the
// three-way rule against the artifact_download declaration.
func artifactNext(ref string, task *iagents.AgentTask, spec *iagents.AgentSpec, given map[string]string) []output.NextAction {
	if spec == nil || !task.IsTerminal || len(task.Artifacts) == 0 {
		return nil
	}
	if op, ok := spec.Op(iagents.VerbArtifactDownload); !ok || !op.Wired {
		return nil
	}
	dlArgs, _ := paramArgsFor(spec, iagents.VerbArtifactDownload, given)
	var next []output.NextAction
	for _, a := range task.Artifacts {
		if a.ID == "" || !safeNextID(a.ID) {
			continue // a server id failing the whitelist: skip this artifact, do not risk injection
		}
		next = append(next, output.NextAction{
			// Only whitelisted ids are interpolated; the artifact name is
			// agent-controlled text and stays out of the label.
			Label:    fmt.Sprintf("Download artifact %s", a.ID),
			Command:  fmt.Sprintf("lark-cli agents task get %s %s --artifact %s -o <save_path>%s", ref, task.TaskID, a.ID, dlArgs),
			Template: true,
		})
	}
	return next
}

// defaultWatchTimeout is the bounded poll window meta.next suggests for a
// still-running task. On expiry the poll returns the current state (exit 0) plus
// a fresh watch hint, so the caller re-watches in segments instead of blocking
// once; `--watch` alone (--timeout 0) stays unbounded for compatibility.
//
// 90s, not 30s: a real backend task often runs ~60s, and a 30s window forced a
// caller to re-issue --watch 2-3 times. Each new process restarts the poll
// backoff from 1s, so the back-to-back calls left no gap and self-hammered the
// backend into a rate_limit. A task finishing earlier still returns immediately,
// since --watch stops at any terminal state.
const defaultWatchTimeout = 90 * time.Second
