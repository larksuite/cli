// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/cmd/event/render"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/transport"
	appconsume "github.com/larksuite/cli/internal/event/application/consume"
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

type consumeCmdOpts struct {
	params    []string
	jqExpr    string
	quiet     bool
	outputDir string

	maxEvents int
	timeout   time.Duration
	dryRun    bool
}

func NewCmdConsume(f *cmdutil.Factory, snap *catalog.Snapshot) *cobra.Command {
	var o consumeCmdOpts

	cmd := &cobra.Command{
		Use:   "consume <EventKey>",
		Short: "Start consuming events for an EventKey",
		Long: `Start consuming real-time events for the given EventKey.

The consume command connects to the event bus daemon (starting it if needed),
subscribes to the specified EventKey, and streams processed events to stdout.

Output is one JSON object per line (NDJSON). Pipe through 'jq .' if you need
pretty-printed formatting.

Use 'event list' to see all available EventKeys.
Use 'event schema <EventKey>' for parameter details.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConsume(cmd, f, snap, args[0], o)
		},
	}

	cmd.Flags().StringArrayVarP(&o.params, "param", "p", nil, "Key=value parameter (repeatable)")
	cmd.Flags().StringVar(&o.jqExpr, "jq", "", "JQ expression to filter output")
	cmd.Flags().BoolVar(&o.quiet, "quiet", false, "Suppress routine and per-event stderr output, including ready/exit markers and drop diagnostics. This can hide event loss; omit --quiet when integrity matters")
	cmd.Flags().StringVar(&o.outputDir, "output-dir", "", "Write each event as a file in this directory (within the allowed roots: cwd, /tmp, ~/files; denylisted system/credential paths are rejected)")
	cmd.Flags().IntVar(&o.maxEvents, "max-events", 0, "Exit after N successful emits (0 = unlimited). Multi-worker EventKeys may emit up to workers-1 past N before all workers stop. Bounded runs ignore stdin EOF.")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "Decide and preview the consume (identity, preconditions, side effects) without performing any of them, then exit")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 0, "Exit after DURATION (e.g. 30s, 2m). 0 = no timeout. Timeout is a normal exit (code 0; stderr 'reason: timeout'). Bounded runs ignore stdin EOF.")
	cmd.Flags().String("as", "auto", "identity type: user | bot | auto (must match EventKey's declared AuthTypes)")
	_ = cmd.RegisterFlagCompletionFunc("as", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"user", "bot", "auto"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func runConsume(cmd *cobra.Command, f *cmdutil.Factory, snap *catalog.Snapshot, eventKey string, o consumeCmdOpts) error {
	// Pipe-close (e.g. `... | head -n 1`) must reach the EPIPE error path in the loop, not SIGPIPE-kill.
	ignoreBrokenPipe()

	cfg, err := f.Config()
	if err != nil {
		return err
	}

	paramMap, err := parseParams(o.params)
	if err != nil {
		return err
	}

	entry, ok := snap.Resolve(eventKey)
	if !ok {
		return unknownEventKeyErr(snap, eventKey)
	}
	keyDef := entry.Definition()

	identity, err := resolveIdentity(cmd, f, keyDef)
	if err != nil {
		return err
	}

	if o.jqExpr != "" {
		if err := output.ValidateJqExpression(o.jqExpr); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
				WithParam("--jq").
				WithCause(err).
				WithHint("see `lark-cli event consume --help` EXAMPLES for common patterns, or `lark-cli event schema %s` for valid field paths", eventKey)
		}
	}

	outputDir := o.outputDir
	if outputDir != "" {
		safePath, err := sanitizeOutputDir(outputDir)
		if err != nil {
			return err
		}
		outputDir = safePath
	}

	domain := core.ResolveEndpoints(cfg.Brand).Open

	// Surface auth errors before forking the bus daemon. A dry run instead
	// reports the unusable credential as a blocked precondition: the caller
	// asked what would happen, and "a real run would refuse to authenticate"
	// is a legitimate part of that answer.
	var tokenErr error
	if _, err := resolveTenantToken(cmd.Context(), f, cfg.AppID); err != nil {
		if !o.dryRun {
			return err
		}
		tokenErr = err
	}

	apiClient, err := f.NewAPIClient()
	if err != nil {
		return err
	}
	runtime := &consumeRuntime{client: apiClient, accessIdentity: identity}
	// botRuntime pins AsBot: /app_versions rejects UAT (99991668) and /connection is app-level.
	botRuntime := &consumeRuntime{client: apiClient, accessIdentity: core.AsBot}

	// Weak-dependency fetch: failures leave appVer==nil and downgrade preflight to a no-op.
	preflightErrOut := f.IOStreams.ErrOut
	if o.quiet {
		preflightErrOut = io.Discard
	}
	appVer, appVerErr := appmeta.FetchCurrentPublished(cmd.Context(), botRuntime, cfg.AppID)
	switch {
	case appVerErr != nil:
		fmt.Fprintf(preflightErrOut, "[event] skipped console precheck: %s\n", describeAppMetaErr(appVerErr))
	case appVer == nil:
		fmt.Fprintln(preflightErrOut, "[event] skipped console precheck: app has no published version")
	}

	// Callback subscriptions live in application/get, not app_versions; fetch the
	// callback 底账 only for callback-type EventKeys. Weak dependency: on error,
	// leave subscribedCallbacks nil so the callback precheck skips.
	var subscribedCallbacks []string
	if keyDef.SubscriptionType == eventlib.SubTypeCallback {
		cbs, cbErr := appmeta.FetchSubscribedCallbacks(cmd.Context(), botRuntime, cfg.AppID)
		if cbErr != nil {
			fmt.Fprintf(preflightErrOut, "[event] skipped console precheck: %s\n", describeAppMetaErr(cbErr))
		} else {
			subscribedCallbacks = cbs
		}
	}

	pf := &preflightCtx{
		factory:             f,
		appID:               cfg.AppID,
		brand:               cfg.Brand,
		eventKey:            eventKey,
		identity:            identity,
		keyDef:              keyDef,
		appVer:              appVer,
		subscribedCallbacks: subscribedCallbacks,
	}

	svc := &appconsume.Service{
		Strategies: consumeStrategies,
		Identity:   identityResolverFunc(func(context.Context, *catalog.Entry) (string, error) { return string(identity), nil }),
		Preflight: preflightReaderFunc(func(ctx context.Context, _ *catalog.Entry, _ string) ([]appconsume.Precondition, error) {
			return readPreconditions(ctx, pf, appVerErr, tokenErr), nil
		}),
	}
	req := appconsume.Request{
		EventKey:  eventKey,
		Params:    paramMap,
		JQExpr:    o.jqExpr,
		OutputDir: outputDir,
		DryRun:    o.dryRun,
		MaxEvents: o.maxEvents,
		Timeout:   o.timeout,
		IsTTY:     f.IOStreams.IsTerminal,
	}
	decision, err := svc.Decide(cmd.Context(), entry, req, appconsume.ExecutionContext{API: runtime})
	if err != nil {
		return err
	}

	if o.dryRun {
		return render.WriteDecisionJSON(f.IOStreams.Out, f.IOStreams.ErrOut, string(identity), decision.View())
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			if !o.quiet && f.IOStreams.IsTerminal {
				fmt.Fprintln(f.IOStreams.ErrOut, "\nShutting down...")
			}
			cancel()
		case <-ctx.Done():
		}
	}()

	errOut := f.IOStreams.ErrOut
	if o.quiet {
		errOut = io.Discard
	}

	runner := streamRunnerFunc(func(ctx context.Context, prepare appconsume.PrepareFunc) error {
		// Non-TTY unbounded consumers use stdin EOF as shutdown for subprocess
		// callers. Bounded runs already have --max-events/--timeout as their
		// lifecycle control.
		//
		// The watcher starts here rather than before the decision is executed:
		// a blocked decision never reaches this point, and starting it earlier
		// announced "stdin closed — shutting down" on a run that was actually
		// refused for an unmet precondition, pointing the caller at the wrong
		// cause.
		if shouldWatchStdinEOF(f.IOStreams.IsTerminal, o.maxEvents, o.timeout) {
			watchStdinEOF(os.Stdin, cancel, errOut)
		}

		return consume.Run(ctx, transport.New(), cfg.AppID, cfg.ProfileName, domain,
			applyDecision(consume.Options{
				EventKey:        eventKey,
				Def:             keyDef,
				JQExpr:          o.jqExpr,
				Quiet:           o.quiet,
				OutputDir:       outputDir,
				Runtime:         runtime,
				Out:             f.IOStreams.Out,
				ErrOut:          errOut,
				RemoteAPIClient: botRuntime,
				MaxEvents:       o.maxEvents,
				Timeout:         o.timeout,
				IsTTY:           f.IOStreams.IsTerminal,
			}, decision, prepare))
	})
	return svc.Execute(ctx, entry, decision, runner, appconsume.ExecutionContext{API: runtime})
}

// applyDecision transfers the decided parts of a consume onto the host's
// options. It exists as a named function because these three assignments are
// couplings the command alone can get wrong, and a test can only pin them where
// they are written.
//
// The parameters and the flag travel together: the deciding layer already ran
// the normalizer on exactly these values to compute the subscription identity.
// The flag without the values would leave the host normalizing input the bus
// was never told about; the values without the flag would run a
// once-per-consumer hook a second time. Prepare carries the strategy the
// decision settled on, so what was decided is what executes instead of the
// declaration's own hook.
func applyDecision(opts consume.Options, decision *appconsume.Decision, prepare appconsume.PrepareFunc) consume.Options {
	opts.Params = decision.NormalizedParams()
	opts.ParamsNormalized = true
	opts.Prepare = prepare
	return opts
}

// resolveIdentity resolves the session identity and enforces keyDef.AuthTypes as a whitelist.
func resolveIdentity(cmd *cobra.Command, f *cmdutil.Factory, keyDef *eventlib.KeyDefinition) (core.Identity, error) {
	flagAs := core.Identity(cmd.Flag("as").Value.String())
	identity := f.ResolveAs(cmd.Context(), cmd, flagAs)
	if len(keyDef.AuthTypes) > 0 {
		if err := f.CheckIdentity(identity, keyDef.AuthTypes); err != nil {
			return "", err
		}
	}
	return identity, nil
}

type preflightCtx struct {
	factory  *cmdutil.Factory
	appID    string
	brand    core.LarkBrand
	eventKey string
	identity core.Identity
	keyDef   *eventlib.KeyDefinition
	appVer   *appmeta.AppVersion
	// subscribedCallbacks is the application/get 底账 for callback-type EventKeys;
	// nil means "not fetched / unavailable" → callback precheck skips (weak dependency).
	subscribedCallbacks []string
}

// preflightScopes compares required scopes against session-available scopes
// (user: UAT stored; bot: appVer.TenantScopes). checked reports whether a
// comparison actually happened: "the ledger was unavailable" and "the check
// passed" are different answers, and only the caller can decide how loudly to
// say the first one.
func preflightScopes(ctx context.Context, pf *preflightCtx) (checked bool, err error) {
	if len(pf.keyDef.Scopes) == 0 || pf.identity == "" {
		return true, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var storedScopes string
	switch {
	case pf.identity.IsBot():
		if pf.appVer == nil {
			return false, nil
		}
		storedScopes = strings.Join(pf.appVer.TenantScopes, " ")
	case pf.identity == core.AsUser:
		result, err := pf.factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(pf.identity, pf.appID))
		if err != nil || result == nil || result.Scopes == "" {
			return false, nil //nolint:nilerr // best-effort: the bus handshake surfaces the real auth error
		}
		storedScopes = result.Scopes
	default:
		return false, nil
	}

	missing := auth.MissingScopes(storedScopes, pf.keyDef.Scopes)
	if len(missing) == 0 {
		return true, nil
	}
	permissionErr := errs.NewPermissionError(errs.SubtypeMissingScope,
		"missing required scopes for EventKey %s (as %s): %s",
		pf.eventKey, pf.identity, strings.Join(missing, ", ")).
		WithIdentity(string(pf.identity)).
		WithMissingScopes(missing...)
	if pf.identity.IsBot() {
		permissionErr.WithHint("%s", botScopeRemediationHint(pf.brand, pf.appID, missing))
	}
	// The scope check itself completed, so the precondition is answered even
	// though it answered "missing". A user-identity hint is deliberately left
	// unset: the root presenter generates it from the identity and missing
	// scopes, projected onto the commands this distribution actually ships.
	return true, permissionErr
}

// scopeRemediationHint returns an identity-appropriate fix for missing scopes.
// The bot-specific scan-to-enable link adds the scopes to the app manifest,
// after which the tenant token carries them. User recovery is generated from
// the PermissionError's identity and missing_scopes by the root presenter.
func botScopeRemediationHint(brand core.LarkBrand, appID string, missing []string) string {
	return fmt.Sprintf("grant these scopes by scanning: %s",
		addonsHintURL(brand, appID, missingScopeAddons(core.AsBot, missing)))
}

// preflightEventTypes verifies every RequiredConsoleEvents entry is subscribed
// in the app's console 底账 — published app_versions for event subscriptions,
// application/get subscribed_callbacks for callback subscriptions.
func preflightEventTypes(pf *preflightCtx) error {
	if len(pf.keyDef.RequiredConsoleEvents) == 0 {
		return nil
	}

	var subscribed []string
	noun := "event types"
	if pf.keyDef.SubscriptionType == eventlib.SubTypeCallback {
		if pf.subscribedCallbacks == nil {
			return nil
		}
		subscribed = pf.subscribedCallbacks
		noun = "callbacks"
	} else {
		if pf.appVer == nil {
			return nil
		}
		subscribed = pf.appVer.EventTypes
	}

	have := make(map[string]bool, len(subscribed))
	for _, t := range subscribed {
		have[t] = true
	}
	var missing []string
	for _, t := range pf.keyDef.RequiredConsoleEvents {
		if !have[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	url := addonsHintURL(pf.brand, pf.appID, missingSubscriptionAddons(pf.keyDef.SubscriptionType, pf.identity, missing))
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"EventKey %s requires %s not subscribed in console: %s",
		pf.keyDef.Key, noun, strings.Join(missing, ", ")).
		WithHint("subscribe these %s by scanning: %s", noun, url)
}

// sanitizeOutputDir defers to the built-in path policy (SafeOutputPath):
// allowed roots are cwd, /tmp, and ~/files; the denylist wins over all.
func sanitizeOutputDir(dir string) (string, error) {
	safe, err := validate.SafeOutputPath(dir)
	if err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s %q: %s", errOutputDirUnsafe, dir, err).
			WithParam("--output-dir").
			WithCause(errOutputDirUnsafe)
	}
	return safe, nil
}

// resolveTenantToken fetches the app's tenant access token.
func resolveTenantToken(ctx context.Context, f *cmdutil.Factory, appID string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsBot, appID))
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return "", err
		}
		return "", errs.NewAuthenticationError(errs.SubtypeTokenMissing,
			"resolve tenant access token: %s", err).WithCause(err)
	}
	if result == nil || result.Token == "" {
		return "", errs.NewAuthenticationError(errs.SubtypeTokenMissing,
			"no tenant access token available for app %s", appID).
			WithHint("check that app_secret is configured for this distribution")
	}
	return result.Token, nil
}

// Sentinels for errors.Is checks; call sites wrap them as typed ValidationError causes.
var (
	errInvalidParamFormat = errors.New("invalid --param format") //nolint:forbidigo // sentinel, typed at call sites
	errOutputDirUnsafe    = errors.New("unsafe --output-dir")    //nolint:forbidigo // sentinel, typed at call sites
)

func parseParams(raw []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s %q: expected key=value", errInvalidParamFormat, kv).
				WithParam("--param").
				WithCause(errInvalidParamFormat)
		}
		m[k] = v
	}
	return m, nil
}

// watchStdinEOF drains r until EOF, writes a diagnostic, then cancels; only safe in non-TTY mode.
func watchStdinEOF(r io.Reader, cancel context.CancelFunc, errOut io.Writer) {
	go func() {
		_, _ = io.Copy(io.Discard, r)
		fmt.Fprintln(errOut, "[event] stdin closed — shutting down. "+
			"consume treats stdin EOF as exit signal (wired for AI subprocess callers). "+
			"To keep running: pass --max-events/--timeout for bounded run, "+
			"or keep stdin open (e.g. `< /dev/tty` interactive, `< <(tail -f /dev/null)` script), "+
			"or stop via SIGTERM instead of closing stdin.")
		cancel()
	}()
}

// shouldWatchStdinEOF gates the stdin-EOF shutdown watcher: non-TTY unbounded runs only (<= 0 mirrors downstream's >0-is-bounded semantics, so negative bounds stay unbounded).
func shouldWatchStdinEOF(isTerminal bool, maxEvents int, timeout time.Duration) bool {
	return !isTerminal && maxEvents <= 0 && timeout <= 0
}
