// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package consumecli owns command-facing event consume setup shared by
// `event consume` and compatibility shortcuts.
package consumecli

import (
	"context"
	"encoding/json"
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
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

type Options struct {
	Params         []string
	ParamMap       map[string]string
	InternalParams map[string]string
	JQExpr         string
	Quiet          bool
	OutputDir      string

	MaxEvents int
	Timeout   time.Duration

	IdentityOverride core.Identity
	Envelope         *consume.OutputEnvelope
	WatchStdinEOF    bool
}

func Run(cmd *cobra.Command, f *cmdutil.Factory, eventKey string, o Options) error {
	// Pipe-close (e.g. `... | head -n 1`) must reach the EPIPE error path in the loop, not SIGPIPE-kill.
	ignoreBrokenPipe()

	cfg, err := f.Config()
	if err != nil {
		return err
	}

	paramMap, err := paramsMap(o)
	if err != nil {
		return err
	}

	keyDef, ok := eventlib.Lookup(eventKey)
	if !ok {
		return UnknownEventKeyErr(eventKey)
	}

	identity, err := resolveIdentity(cmd, f, keyDef, o.IdentityOverride)
	if err != nil {
		return err
	}

	if o.JQExpr != "" {
		if err := output.ValidateJqExpression(o.JQExpr); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
				WithParam("--jq").
				WithCause(err).
				WithHint("see `lark-cli event consume --help` EXAMPLES for common patterns, or `lark-cli event schema %s` for valid field paths", eventKey)
		}
	}

	outputDir := o.OutputDir
	if outputDir != "" {
		safePath, err := SanitizeOutputDir(outputDir)
		if err != nil {
			return err
		}
		outputDir = safePath
	}

	domain := core.ResolveEndpoints(cfg.Brand).Open

	// Surface auth errors before forking the bus daemon.
	if _, err := ResolveTenantToken(cmd.Context(), f, cfg.AppID); err != nil {
		return err
	}

	apiClient, err := f.NewAPIClient()
	if err != nil {
		return err
	}
	runtime := &consumeRuntime{client: apiClient, accessIdentity: identity}
	// botRuntime pins AsBot: /app_versions rejects UAT (99991668) and /connection is app-level.
	botRuntime := &consumeRuntime{client: apiClient, accessIdentity: core.AsBot}

	preflightErrOut := f.IOStreams.ErrOut
	if o.Quiet {
		preflightErrOut = io.Discard
	}
	appVer, appVerErr := appmeta.FetchCurrentPublished(cmd.Context(), botRuntime, cfg.AppID)
	switch {
	case appVerErr != nil:
		fmt.Fprintf(preflightErrOut, "[event] skipped console precheck: %s\n", describeAppMetaErr(appVerErr))
	case appVer == nil:
		fmt.Fprintln(preflightErrOut, "[event] skipped console precheck: app has no published version")
	}

	pf := &PreflightCtx{
		Factory:  f,
		AppID:    cfg.AppID,
		Brand:    cfg.Brand,
		EventKey: eventKey,
		Identity: identity,
		KeyDef:   keyDef,
		Params:   paramMap,
		AppVer:   appVer,
	}
	if err := PreflightEventTypes(pf); err != nil {
		return err
	}
	if err := PreflightScopes(cmd.Context(), pf); err != nil {
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
			if !o.Quiet && f.IOStreams.IsTerminal {
				fmt.Fprintln(f.IOStreams.ErrOut, "\nShutting down...")
			}
			cancel()
		case <-ctx.Done():
		}
	}()

	errOut := f.IOStreams.ErrOut
	if o.Quiet {
		errOut = io.Discard
	}

	if o.WatchStdinEOF && ShouldWatchStdinEOF(f.IOStreams.IsTerminal, o.MaxEvents, o.Timeout) {
		WatchStdinEOF(os.Stdin, cancel, errOut)
	}

	return consume.Run(ctx, transport.New(), cfg.AppID, cfg.ProfileName, domain, consume.Options{
		EventKey:        eventKey,
		Params:          paramMap,
		InternalParams:  o.InternalParams,
		JQExpr:          o.JQExpr,
		Quiet:           o.Quiet,
		OutputDir:       outputDir,
		Runtime:         runtime,
		Out:             f.IOStreams.Out,
		ErrOut:          errOut,
		RemoteAPIClient: botRuntime,
		MaxEvents:       o.MaxEvents,
		Timeout:         o.Timeout,
		IsTTY:           f.IOStreams.IsTerminal,
		Envelope:        o.Envelope,
	})
}

func paramsMap(o Options) (map[string]string, error) {
	m := make(map[string]string, len(o.ParamMap)+len(o.Params))
	for k, v := range o.ParamMap {
		m[k] = v
	}
	parsed, err := ParseParams(o.Params)
	if err != nil {
		return nil, err
	}
	for k, v := range parsed {
		m[k] = v
	}
	return m, nil
}

func resolveIdentity(cmd *cobra.Command, f *cmdutil.Factory, keyDef *eventlib.KeyDefinition, override core.Identity) (core.Identity, error) {
	identity := override
	if identity == "" {
		flagAs := core.AsAuto
		if cmd != nil && cmd.Flag("as") != nil {
			flagAs = core.Identity(cmd.Flag("as").Value.String())
		}
		identity = f.ResolveAs(cmd.Context(), cmd, flagAs)
	}
	if len(keyDef.AuthTypes) > 0 {
		if err := f.CheckIdentity(identity, keyDef.AuthTypes); err != nil {
			return "", err
		}
	}
	return identity, nil
}

type PreflightCtx struct {
	Factory  *cmdutil.Factory
	AppID    string
	Brand    core.LarkBrand
	EventKey string
	Identity core.Identity
	KeyDef   *eventlib.KeyDefinition
	Params   map[string]string
	AppVer   *appmeta.AppVersion
}

func PreflightScopes(ctx context.Context, pf *PreflightCtx) error {
	scopes := pf.KeyDef.Scopes
	if pf.KeyDef.ScopesForParams != nil {
		scopes = pf.KeyDef.ScopesForParams(pf.Params)
	}
	if len(scopes) == 0 || pf.Identity == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var storedScopes string
	switch {
	case pf.Identity.IsBot():
		if pf.AppVer == nil {
			return nil
		}
		storedScopes = strings.Join(pf.AppVer.TenantScopes, " ")
	case pf.Identity == core.AsUser:
		result, err := pf.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(pf.Identity, pf.AppID))
		if err != nil || result == nil || result.Scopes == "" {
			return nil //nolint:nilerr // best-effort: bus handshake will surface real auth error
		}
		storedScopes = result.Scopes
	default:
		return nil
	}

	missing := auth.MissingScopes(storedScopes, scopes)
	if len(missing) == 0 {
		return nil
	}
	return errs.NewPermissionError(errs.SubtypeMissingScope,
		"missing required scopes for EventKey %s (as %s): %s",
		pf.EventKey, pf.Identity, strings.Join(missing, ", ")).
		WithIdentity(string(pf.Identity)).
		WithMissingScopes(missing...).
		WithHint("%s", ScopeRemediationHint(pf.Brand, pf.AppID, pf.Identity, missing))
}

func ScopeRemediationHint(brand core.LarkBrand, appID string, identity core.Identity, missing []string) string {
	if identity.IsBot() {
		return fmt.Sprintf("grant these scopes by scanning: %s",
			addonsHintURL(brand, appID, missingScopeAddons(identity, missing)))
	}
	return fmt.Sprintf(
		"run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.",
		strings.Join(missing, " "))
}

func PreflightEventTypes(pf *PreflightCtx) error {
	if len(pf.KeyDef.RequiredConsoleEvents) == 0 {
		return nil
	}

	if pf.AppVer == nil {
		return nil
	}
	subscribed := pf.AppVer.EventTypes

	have := make(map[string]bool, len(subscribed))
	for _, t := range subscribed {
		have[t] = true
	}
	var missing []string
	for _, t := range pf.KeyDef.RequiredConsoleEvents {
		if !have[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	url := addonsHintURL(pf.Brand, pf.AppID, missingSubscriptionAddons(pf.Identity, missing))
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"EventKey %s requires event types not subscribed in console: %s",
		pf.KeyDef.Key, strings.Join(missing, ", ")).
		WithHint("subscribe these event types by scanning: %s", url)
}

func SanitizeOutputDir(dir string) (string, error) {
	if strings.HasPrefix(dir, "~") {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s; use a relative path like ./output instead", ErrOutputDirTilde).
			WithParam("--output-dir").
			WithCause(ErrOutputDirTilde)
	}
	safe, err := validate.SafeOutputPath(dir)
	if err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s %q: %s", ErrOutputDirUnsafe, dir, err).
			WithParam("--output-dir").
			WithCause(err)
	}
	return safe, nil
}

func ResolveTenantToken(ctx context.Context, f *cmdutil.Factory, appID string) (string, error) {
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

var (
	ErrInvalidParamFormat = errors.New("invalid --param format")                    //nolint:forbidigo // sentinel, typed at call sites
	ErrOutputDirTilde     = errors.New("--output-dir does not support ~ expansion") //nolint:forbidigo // sentinel, typed at call sites
	ErrOutputDirUnsafe    = errors.New("unsafe --output-dir")                       //nolint:forbidigo // sentinel, typed at call sites
)

func ParseParams(raw []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"%s %q: expected key=value", ErrInvalidParamFormat, kv).
				WithParam("--param").
				WithCause(ErrInvalidParamFormat)
		}
		m[k] = v
	}
	return m, nil
}

func WatchStdinEOF(r io.Reader, cancel context.CancelFunc, errOut io.Writer) {
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

func ShouldWatchStdinEOF(isTerminal bool, maxEvents int, timeout time.Duration) bool {
	return !isTerminal && maxEvents <= 0 && timeout <= 0
}

type consumeRuntime struct {
	client         *client.APIClient
	accessIdentity core.Identity
}

func (r *consumeRuntime) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	resp, err := r.client.DoAPI(ctx, client.RawApiRequest{
		Method: method,
		URL:    path,
		Data:   body,
		As:     r.accessIdentity,
	})
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"api %s %s: %s", method, path, err).WithCause(err)
	}
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode >= 400 && !client.IsJSONContentType(ct) && ct != "" {
		const maxBodyEcho = 256
		body := string(resp.RawBody)
		if len(body) > maxBodyEcho {
			body = body[:maxBodyEcho] + "...(truncated)"
		}
		if resp.StatusCode >= 500 {
			return nil, errs.NewNetworkError(errs.SubtypeNetworkServer,
				"api %s %s returned %d: %s", method, path, resp.StatusCode, body).WithRetryable()
		}
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"api %s %s returned %d: %s", method, path, resp.StatusCode, body)
	}
	result, err := client.ParseJSONResponse(resp)
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"api %s %s: %s", method, path, err).WithCause(err)
	}
	if apiErr := r.client.CheckResponse(result, r.accessIdentity); apiErr != nil {
		return json.RawMessage(resp.RawBody), apiErr
	}
	return json.RawMessage(resp.RawBody), nil
}
