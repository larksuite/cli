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

// consumeCmdOpts bundles the flag-backed inputs for `event consume`.
// Kept as a struct so runConsume's signature stays short and new flags
// don't balloon its parameter list.
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
	cmd.Flags().StringVar(&o.outputDir, "output-dir", "", "Write each event as a file in this directory")
	cmd.Flags().IntVar(&o.maxEvents, "max-events", 0, "Exit after N successful emits (0 = unlimited). Multi-worker EventKeys may emit up to workers-1 past N before all workers stop.")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 0, "Exit after DURATION (e.g. 30s, 2m). 0 = no timeout. Timeout is a normal exit (code 0; stderr 'reason: timeout').")
	cmd.Flags().String("as", "auto", "identity type: user | bot | auto (must match EventKey's declared AuthTypes)")
	_ = cmd.RegisterFlagCompletionFunc("as", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"user", "bot", "auto"}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runConsume(cmd *cobra.Command, f *cmdutil.Factory, eventKey string, o consumeCmdOpts) error {
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
			return err
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

	// Verify a tenant access token can be obtained up front. The forked bus
	// daemon will resolve its own TAT from profile+keychain; we throw this
	// one away. Failing here surfaces auth errors before we fork and crash.
	if _, err := resolveTenantToken(cmd.Context(), f, cfg.AppID); err != nil {
		return err
	}

	apiClient, err := f.NewAPIClient()
	if err != nil {
		// Pass the inner error through verbatim: f.NewAPIClient can return
		// *core.ConfigError (Code=2 + actionable Hint "run lark-cli config
		// init…") or auth-related errors — wrapping would force ExitInternal
		// and strip the hint, turning "not configured" into "internal error".
		return err
	}
	runtime := &consumeRuntime{client: apiClient, accessIdentity: identity}
	// botRuntime pins identity=AsBot regardless of --as so preflight probes
	// (app_versions, event/v1/connection) always use TAT. /app_versions
	// rejects UAT with 99991668, and /connection is an app-level query.
	botRuntime := &consumeRuntime{client: apiClient, accessIdentity: core.AsBot}

	// Weak-dependency fetch of the app's current published version. Failures
	// (210508 insufficient permission, network errors, never-published apps)
	// leave appVer == nil and we downgrade preflight checks to a no-op with a
	// stderr breadcrumb. Blocking startup here would be worse than letting a
	// misconfigured subscription surface later — the OAPI access requirement
	// is a soft one.
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

	pf := &preflightCtx{
		factory:  f,
		appID:    cfg.AppID,
		brand:    cfg.Brand,
		eventKey: eventKey,
		identity: identity,
		keyDef:   keyDef,
		appVer:   appVer,
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
	// Release the signal subscription when runConsume returns — without
	// this, repeated invocations in the same process would leak the
	// notify channel and the goroutine parked on <-sigCh.
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			if !o.quiet && f.IOStreams.IsTerminal {
				fmt.Fprintln(f.IOStreams.ErrOut, "\nShutting down...")
			}
			cancel()
		case <-ctx.Done():
			// ctx cancelled by another path (timeout, stdin EOF, max-events);
			// exit the goroutine so signal.Stop can clean up.
		}
	}()

	errOut := f.IOStreams.ErrOut
	if o.quiet {
		errOut = io.Discard
	}

	// Non-TTY: a closed stdin is a valid shutdown signal for subprocess
	// callers. In TTY the stdin is the terminal, so we must not treat
	// Ctrl-D as shutdown.
	if !f.IOStreams.IsTerminal {
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
	// consume.Run returning nil means one of the three normal exits: --max-events
	// reached, --timeout elapsed, or user signal (SIGTERM / stdin close / ctrl+C).
	// All three are "success" in the Unix sense — the caller asked for one of
	// these stopping conditions and got it. stderr carries reason=limit|timeout|
	// signal for callers that need to distinguish. Exit code 0.
	return nil
}

// resolveIdentity picks the identity for this session via the shared
// ResolveAs flow (explicit --as > config DefaultAs > auto-detect) and
// then enforces the EventKey's AuthTypes as a strict whitelist. If the
// resolved identity isn't in AuthTypes, CheckIdentity returns an error
// that steers the user toward passing --as explicitly. AuthTypes is
// not a default — it only constrains what's allowed.
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

// preflightCtx bundles the shared state every preflight check reads.
// Grouping avoids the 8-parameter signatures that made the call sites
// unreadable.
type preflightCtx struct {
	factory  *cmdutil.Factory
	appID    string
	brand    core.LarkBrand
	eventKey string
	identity core.Identity
	keyDef   *eventlib.KeyDefinition
	appVer   *appmeta.AppVersion
}

// preflightScopes compares the required scopes for an EventKey against the
// scopes actually available to the session and surfaces an actionable hint
// when any are missing. Scope source depends on identity:
//
//   - user → stored.Scope from the UAT keychain record (the ground truth
//     for "what the user actually consented to"). Skip check when stored
//     record is missing.
//   - bot  → the tenant scopes from the app's current published version
//     (appVer.TenantScopes). Skip check when appVer is nil (the
//     app_versions OAPI failed or the app has never been published) —
//     weak dependency: we don't want a flaky preflight to block consume.
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
			return nil
		}
		storedScopes = result.Scopes
	default:
		return nil
	}

	missing := auth.MissingScopes(storedScopes, pf.keyDef.Scopes)
	if len(missing) == 0 {
		return nil
	}
	return output.ErrWithHint(
		output.ExitAuth, "auth",
		fmt.Sprintf("missing required scopes for EventKey %s (as %s): %s",
			pf.eventKey, pf.identity, strings.Join(missing, ", ")),
		scopeRemediationHint(pf.identity, missing, pf.appID, pf.brand),
	)
}

// scopeRemediationHint returns the identity-appropriate fix suggestion for a
// set of missing scopes. user gets the auth-login flow (scopes attach to the
// token); bot gets pointed at the developer console (scopes attach to the
// app and require a new published version to take effect). Both paths
// include a deep-link URL so a human or AI reader can act without guessing
// at the command or console navigation.
func scopeRemediationHint(identity core.Identity, missing []string, appID string, brand core.LarkBrand) string {
	if identity.IsBot() {
		return fmt.Sprintf(
			"grant these scopes and publish a new app version at: %s",
			consoleScopeGrantURL(brand, appID, missing),
		)
	}
	return fmt.Sprintf(
		"run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.",
		strings.Join(missing, " "),
	)
}

// preflightEventTypes verifies that every event_type the EventKey depends on
// is actually subscribed in the app's current published version. A mismatch
// means the developer declared the EventKey in code but forgot to subscribe
// (or publish) the upstream event type — the consume would succeed locally
// but never receive anything. This is a configuration error, not auth, so
// ExitValidation is the right exit code.
//
// keyDef.RequiredConsoleEvents drives what's checked — keys that leave it
// empty opt out of the check entirely. The default-to-EventType shortcut
// is intentionally NOT used: declaring the dependency explicitly is a form
// of documentation (especially for processed keys that fan in multiple
// upstreams), and a silent default hides mistakes at registration time.
//
// appVer == nil is a weak-dependency skip: when the app_versions OAPI fails
// (permission denied, network error, or the app was never published) we
// degrade gracefully rather than block startup.
func preflightEventTypes(pf *preflightCtx) error {
	if pf.appVer == nil || len(pf.keyDef.RequiredConsoleEvents) == 0 {
		return nil
	}
	subscribed := make(map[string]bool, len(pf.appVer.EventTypes))
	for _, t := range pf.appVer.EventTypes {
		subscribed[t] = true
	}
	var missing []string
	for _, t := range pf.keyDef.RequiredConsoleEvents {
		if !subscribed[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return output.ErrWithHint(
		output.ExitValidation, "validation",
		fmt.Sprintf("EventKey %s requires event types not subscribed in console: %s",
			pf.keyDef.Key, strings.Join(missing, ", ")),
		fmt.Sprintf("subscribe these events and publish a new app version at: %s",
			consoleEventSubscriptionURL(pf.brand, pf.appID)),
	)
}

// sanitizeOutputDir rejects absolute/parent-escaping paths (mirrors the
// check `mail +watch` does) and explicitly rejects ~ since SafeOutputPath
// treats "~/x" as a literal dir named "~".
func sanitizeOutputDir(dir string) (string, error) {
	if strings.HasPrefix(dir, "~") {
		return "", output.ErrValidation("%s; use a relative path like ./output instead", errOutputDirTilde)
	}
	safe, err := validate.SafeOutputPath(dir)
	if err != nil {
		return "", output.ErrValidation("%s %q: %s", errOutputDirUnsafe, dir, err)
	}
	return safe, nil
}

// resolveTenantToken fetches the app's tenant access token. Used by the
// remote-connection preflight; separately, the bus daemon needs one too.
func resolveTenantToken(ctx context.Context, f *cmdutil.Factory, appID string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsBot, appID))
	if err != nil {
		return "", output.ErrAuth("resolve tenant access token: %s", err)
	}
	if result == nil || result.Token == "" {
		return "", output.ErrWithHint(
			output.ExitAuth, "auth",
			fmt.Sprintf("no tenant access token available for app %s", appID),
			"Check that app_secret is configured (lark-cli config show) and try 'lark-cli auth login'.",
		)
	}
	return result.Token, nil
}

// Sentinel errors for tests so assertions pin on error identity, not
// phrasing. The human-readable message still includes the offending
// value for the user; errors.Is works via ExitError.Unwrap().
var (
	errInvalidParamFormat = errors.New("invalid --param format")
	errOutputDirTilde     = errors.New("--output-dir does not support ~ expansion")
	errOutputDirUnsafe    = errors.New("unsafe --output-dir")
)

// parseParams turns ["key=value", ...] into map[string]string.
func parseParams(raw []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, output.ErrValidation("%s %q: expected key=value", errInvalidParamFormat, kv)
		}
		m[k] = v
	}
	return m, nil
}

// watchStdinEOF launches a goroutine that drains r until EOF or any error,
// writes a diagnostic line to errOut, then invokes cancel. Used to wire
// stdin-close → exit in non-TTY mode so AI subprocess callers can signal
// shutdown by closing the stdin pipe.
//
// The diagnostic is important: classic daemonisation (`< /dev/null`, `nohup`,
// systemd's default `StandardInput=null`) delivers EOF on the first read,
// which makes consume exit immediately — users who expected a long-running
// daemon see "doesn't start" with no obvious cause. The stderr line names
// the cause and points at the workarounds. Pass io.Discard for errOut to
// suppress the line (e.g. under --quiet).
//
// Never called in TTY mode (would turn accidental Ctrl-D into shutdown).
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
