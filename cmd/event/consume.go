// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/appmeta"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/consumecli"
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
	cmd.Flags().IntVar(&o.maxEvents, "max-events", 0, "Exit after N successful emits (0 = unlimited). Multi-worker EventKeys may emit up to workers-1 past N before all workers stop.")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 0, "Exit after DURATION (e.g. 30s, 2m). 0 = no timeout. Timeout is a normal exit (code 0; stderr 'reason: timeout').")
	cmd.Flags().String("as", "auto", "identity type: user | bot | auto (must match EventKey's declared AuthTypes)")
	_ = cmd.RegisterFlagCompletionFunc("as", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"user", "bot", "auto"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func runConsume(cmd *cobra.Command, f *cmdutil.Factory, eventKey string, o consumeCmdOpts) error {
	return consumecli.Run(cmd, f, eventKey, consumecli.Options{
		Params:        o.params,
		JQExpr:        o.jqExpr,
		Quiet:         o.quiet,
		OutputDir:     o.outputDir,
		MaxEvents:     o.maxEvents,
		Timeout:       o.timeout,
		WatchStdinEOF: true,
	})
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
	return output.ErrWithHint(
		output.ExitAuth, "auth",
		fmt.Sprintf("missing required scopes for EventKey %s (as %s): %s",
			pf.eventKey, pf.identity, strings.Join(missing, ", ")),
		scopeRemediationHint(pf.identity, missing, pf.appID, pf.brand),
	)
}

// scopeRemediationHint returns an identity-appropriate fix for missing scopes.
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

// preflightEventTypes verifies every RequiredConsoleEvents entry is subscribed in the app's current published version.
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

// sanitizeOutputDir rejects absolute/parent-escaping paths and ~ (SafeOutputPath treats it as a literal dir name).
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

// resolveTenantToken fetches the app's tenant access token.
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

var (
	errInvalidParamFormat = errors.New("invalid --param format")
	errOutputDirTilde     = errors.New("--output-dir does not support ~ expansion")
	errOutputDirUnsafe    = errors.New("unsafe --output-dir")
)

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
