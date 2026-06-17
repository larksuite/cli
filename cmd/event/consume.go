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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/transport"
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
}

func NewCmdConsume(f *cmdutil.Factory) *cobra.Command {
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
			return runConsume(cmd, f, args[0], o)
		},
	}

	cmd.Flags().StringArrayVarP(&o.params, "param", "p", nil, "Key=value parameter (repeatable)")
	cmd.Flags().StringVar(&o.jqExpr, "jq", "", "JQ expression to filter output")
	cmd.Flags().BoolVar(&o.quiet, "quiet", false, "Suppress informational messages on stderr")
	cmd.Flags().StringVar(&o.outputDir, "output-dir", "", "Write each event as a file in this directory (relative paths only; absolute paths and ~ are rejected to prevent path traversal)")
	cmd.Flags().IntVar(&o.maxEvents, "max-events", 0, "Exit after N successful emits (0 = unlimited). Multi-worker EventKeys may emit up to workers-1 past N before all workers stop. Bounded runs ignore stdin EOF.")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 0, "Exit after DURATION (e.g. 30s, 2m). 0 = no timeout. Timeout is a normal exit (code 0; stderr 'reason: timeout'). Bounded runs ignore stdin EOF.")
	cmd.Flags().String("as", "auto", "identity type: user | bot | auto (must match EventKey's declared AuthTypes)")
	_ = cmd.RegisterFlagCompletionFunc("as", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"user", "bot", "auto"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func runConsume(cmd *cobra.Command, f *cmdutil.Factory, eventKey string, o consumeCmdOpts) error {
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

	keyDef, ok := eventlib.Lookup(eventKey)
	if !ok {
		return unknownEventKeyErr(eventKey)
	}

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

	// Surface auth errors before forking the bus daemon.
	if _, err := resolveTenantToken(cmd.Context(), f, cfg.AppID); err != nil {
		return err
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
	if err := preflightEventTypes(pf); err != nil {
		return err
	}
	if err := preflightScopes(cmd.Context(), pf); err != nil {
		return err
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

	// Non-TTY unbounded consumers use stdin EOF as shutdown for subprocess callers.
	// Bounded runs already have --max-events/--timeout as their lifecycle control.
	if shouldWatchStdinEOF(f.IOStreams.IsTerminal, o.maxEvents, o.timeout) {
		watchStdinEOF(os.Stdin, cancel, errOut)
	}

	if err := consume.Run(ctx, transport.New(), cfg.AppID, cfg.ProfileName, domain, consume.Options{
		EventKey:        eventKey,
		Params:          paramMap,
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
	}); err != nil {
		return err
	}
	return nil
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

// preflightScopes compares required scopes against session-available scopes (user: UAT stored; bot: appVer.TenantScopes).
func preflightScopes(ctx context.Context, pf *preflightCtx) error {
	if len(pf.keyDef.Scopes) == 0 || pf.identity == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var storedScopes string
	switch {
	case pf.identity.IsBot():
		if pf.appVer == nil {
			return nil
		}
		storedScopes = strings.Join(pf.appVer.TenantScopes, " ")
	case pf.identity == core.AsUser:
		result, err := pf.factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(pf.identity, pf.appID))
		if err != nil || result == nil || result.Scopes == "" {
			return nil //nolint:nilerr // best-effort: bus handshake will surface real auth error
		}
		storedScopes = result.Scopes
	default:
		return nil
	}

	missing := auth.MissingScopes(storedScopes, pf.keyDef.Scopes)
	if len(missing) == 0 {
		return nil
	}
	return errs.NewPermissionError(errs.SubtypeMissingScope,
		"missing required scopes for EventKey %s (as %s): %s",
		pf.eventKey, pf.identity, strings.Join(missing, ", ")).
		WithIdentity(string(pf.identity)).
		WithMissingScopes(missing...).
		WithHint("%s", scopeRemediationHint(pf.brand, pf.appID, pf.identity, missing))
}

// scopeRemediationHint returns an identity-appropriate fix for missing scopes.
// Bot: the scan-to-enable link adds the scopes to the app manifest, after which
// the tenant token carries them. User: the scan link only updates the app
// manifest — the user's own token still lacks the scopes until it is
// re-authorized — so direct the user to re-login instead.
func scopeRemediationHint(brand core.LarkBrand, appID string, identity core.Identity, missing []string) string {
	if identity.IsBot() {
		return fmt.Sprintf("grant these scopes by scanning: %s",
			addonsHintURL(brand, appID, missingScopeAddons(identity, missing)))
	}
	return fmt.Sprintf(
		"run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.",
		strings.Join(missing, " "))
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

// sanitizeOutputDir rejects absolute/parent-escaping paths and ~ (SafeOutputPath treats it as a literal dir name).
func sanitizeOutputDir(dir string) (string, error) {
	if strings.HasPrefix(dir, "~") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s; use a relative path like ./output instead", errOutputDirTilde).
			WithParam("--output-dir").
			WithCause(errOutputDirTilde)
	}
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
			WithHint("Check that app_secret is configured (lark-cli config show) and try 'lark-cli auth login'.")
	}
	return result.Token, nil
}

// Sentinels for errors.Is checks; call sites wrap them as typed ValidationError causes.
var (
	errInvalidParamFormat = errors.New("invalid --param format")                    //nolint:forbidigo // sentinel, typed at call sites
	errOutputDirTilde     = errors.New("--output-dir does not support ~ expansion") //nolint:forbidigo // sentinel, typed at call sites
	errOutputDirUnsafe    = errors.New("unsafe --output-dir")                       //nolint:forbidigo // sentinel, typed at call sites
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
