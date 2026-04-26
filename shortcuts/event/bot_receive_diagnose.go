// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const defaultDiagnoseEventType = "im.message.receive_v1"

// Seams for testing — keep package-level vars so tests can stub out the
// real network probes without touching the shortcut wiring.
var (
	botReceiveProbeWebsocket = diagnoseBotReceiveWebsocket
)

type botReceiveCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type botReceiveSummary struct {
	OK    bool `json:"ok"`
	Pass  int  `json:"pass"`
	Warn  int  `json:"warn"`
	Fail  int  `json:"fail"`
	Skip  int  `json:"skip"`
	Total int  `json:"total"`
}

type botReceivePayload struct {
	EventType string            `json:"event_type"`
	Summary   botReceiveSummary `json:"summary"`
	Checks    []botReceiveCheck `json:"checks"`
	NextSteps []string          `json:"next_steps,omitempty"`
}

type silentSDKLogger struct{}

func (l *silentSDKLogger) Debug(_ context.Context, _ ...interface{}) {}
func (l *silentSDKLogger) Info(_ context.Context, _ ...interface{})  {}
func (l *silentSDKLogger) Warn(_ context.Context, _ ...interface{})  {}
func (l *silentSDKLogger) Error(_ context.Context, _ ...interface{}) {}

var _ larkcore.Logger = (*silentSDKLogger)(nil)

// BotReceiveDiagnose is a read-only diagnostic shortcut that inspects whether
// a bot is correctly set up to receive events (default: im.message.receive_v1).
// It reuses the event/WebSocket stack shared with `event +subscribe`.
//
// Scopes is intentionally the empty slice rather than a hard-coded scope list:
// bot (tenant access token) credentials do not carry an OAuth-style granted
// scope list, so the shared scope pre-check cannot verify a specific event
// permission for bot identity. Declaring a static scope here would both be a
// no-op today and risk blocking the diagnostic in the future if the shared
// pre-check ever learns to fetch bot scopes — bots *missing* the scope are
// exactly the audience this diagnostic is designed to help. The CLI surfaces
// the required scope for the specific --event-type as an advisory check
// inside Execute instead.
var BotReceiveDiagnose = common.Shortcut{
	Service:     "event",
	Command:     "+bot-receive-diagnose",
	Description: "Diagnose why a bot does not receive events (bot-only, read-only)",
	Risk:        "read",
	Scopes:      []string{},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "offline", Type: "bool", Desc: "skip network and WebSocket checks; only verify local bot configuration and credential presence"},
		{Name: "timeout", Type: "int", Default: "5", Desc: "timeout in seconds for network and WebSocket checks (1-60)"},
		{Name: "event-type", Default: defaultDiagnoseEventType, Desc: "event type to diagnose (default im.message.receive_v1)"},
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Diagnose bot event receive readiness without changing remote state").
			Set("command", "event +bot-receive-diagnose").
			Set("event_type", runtime.Str("event-type")).
			Set("offline", runtime.Bool("offline")).
			Set("timeout_sec", runtime.Int("timeout"))
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if runtime.Int("timeout") < 1 || runtime.Int("timeout") > 60 {
			return output.ErrValidation("--timeout must be an integer between 1 and 60")
		}
		if strings.TrimSpace(runtime.Str("event-type")) == "" {
			return output.ErrValidation("--event-type cannot be empty")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		offline := runtime.Bool("offline")
		eventType := strings.TrimSpace(runtime.Str("event-type"))
		timeout := time.Duration(runtime.Int("timeout")) * time.Second
		checks := make([]botReceiveCheck, 0, 8)

		checks = append(checks, diagnoseBotApp(runtime))

		// App credential check (separate from app_resolved to give clearer hints).
		if runtime.Config == nil {
			checks = append(checks,
				failBotReceiveCheck(
					"app_credential",
					"app configuration is unavailable; cannot inspect app secret",
					"run `lark-cli config init --new` to configure the app before diagnosing bot receive events",
				),
				skipBotReceiveCheck("token_bot", "skipped because app configuration is unavailable"),
				skipBotReceiveCheck("endpoint_open", "skipped because app configuration is unavailable"),
				skipBotReceiveCheck("endpoint_ws", "skipped because app configuration is unavailable"),
			)
		} else if strings.TrimSpace(runtime.Config.AppSecret) == "" {
			checks = append(checks, failBotReceiveCheck(
				"app_credential",
				"app secret is empty",
				"run `lark-cli config init --new` or update the configured app secret before diagnosing bot receive events",
			))
		} else {
			checks = append(checks, passBotReceiveCheck("app_credential", "app credentials are configured"))
		}

		// Token acquisition (with a short cancellable ctx derived from the
		// outer shortcut ctx). Skipped entirely when config is unavailable or
		// offline is requested. endpoint_open reachability is inferred from
		// the token acquisition result below (no raw net/http probe is used —
		// the shortcuts layer forbids direct net/http usage).
		token := ""
		var tokenErr error
		switch {
		case runtime.Config == nil:
			// already reported above
		case offline:
			checks = append(checks, skipBotReceiveCheck("token_bot", "skipped (--offline); local app configuration readiness only"))
		case strings.TrimSpace(runtime.Config.AppSecret) == "":
			checks = append(checks, failBotReceiveCheck(
				"token_bot",
				"cannot acquire bot tenant access token because app secret is empty",
				"run `lark-cli config init --new` or update the configured app secret before diagnosing bot receive events",
			))
		default:
			tokenCtx, tokenCancel := context.WithTimeout(ctx, timeout)
			tok, err := resolveBotTenantToken(tokenCtx, runtime)
			tokenCancel()
			switch {
			case err != nil:
				tokenErr = err
				checks = append(checks, failBotReceiveCheck(
					"token_bot",
					fmt.Sprintf("failed to get bot tenant access token: %v", err),
					"check app id/app secret, app status, and bot-related permissions in the developer console",
				))
			case strings.TrimSpace(tok) == "":
				checks = append(checks, failBotReceiveCheck(
					"token_bot",
					"tenant access token is empty",
					"check app id/app secret and retry",
				))
			default:
				token = tok
				checks = append(checks, passBotReceiveCheck("token_bot", "bot tenant access token acquired successfully"))
			}
		}

		// Network probes. The open-endpoint reachability is inferred from
		// the tenant_access_token acquisition above (which already makes an
		// authenticated HTTPS call via the SDK). The WebSocket probe is its
		// own dedicated check. Both are skipped under --offline or when
		// config is unavailable.
		if runtime.Config == nil {
			// handled above
		} else if offline {
			checks = append(checks,
				skipBotReceiveCheck("endpoint_open", "skipped (--offline)"),
				skipBotReceiveCheck("endpoint_ws", "skipped (--offline)"),
			)
		} else {
			openURL := core.ResolveEndpoints(runtime.Config.Brand).Open
			switch {
			case strings.TrimSpace(runtime.Config.AppSecret) == "":
				checks = append(checks, skipBotReceiveCheck("endpoint_open", "skipped because app secret is empty"))
			case tokenErr != nil:
				checks = append(checks, failBotReceiveCheck(
					"endpoint_open",
					fmt.Sprintf("%s unreachable or unauthorized: %v", openURL, tokenErr),
					"check network, proxy, or firewall settings and retry before diagnosing event delivery",
				))
			case strings.TrimSpace(token) == "":
				checks = append(checks, skipBotReceiveCheck("endpoint_open", "skipped because tenant token probe did not run"))
			default:
				checks = append(checks, passBotReceiveCheck("endpoint_open", openURL+" reachable (verified via tenant_access_token acquisition)"))
			}

			if strings.TrimSpace(token) == "" {
				checks = append(checks, skipBotReceiveCheck("endpoint_ws", "skipped because bot token is unavailable"))
			} else {
				checks = append(checks, botReceiveProbeWebsocket(ctx, runtime, eventType, timeout))
			}
		}

		// Things the CLI cannot verify remotely — surface them as warnings
		// with actionable hints rather than silently passing.
		checks = append(checks,
			warnBotReceiveCheck(
				"event_subscription",
				fmt.Sprintf("CLI cannot verify whether %s is subscribed in the developer console", eventType),
				fmt.Sprintf("verify that event `%s` is added in the app's event subscriptions", eventType),
			),
			deriveScopeAdvisoryCheck(eventType),
			warnBotReceiveCheck(
				"bot_availability",
				"bot availability and target chat visibility cannot be inferred locally",
				"confirm the bot is enabled, visible to the target users, and already added to the target chat",
			),
		)

		payload := botReceivePayload{
			EventType: eventType,
			Summary:   summarizeBotReceiveChecks(checks),
			Checks:    checks,
			NextSteps: collectBotReceiveNextSteps(checks),
		}

		runtime.OutFormat(payload, nil, func(w io.Writer) {
			rows := make([]map[string]interface{}, 0, len(checks))
			for _, check := range checks {
				row := map[string]interface{}{
					"check":   check.Name,
					"status":  check.Status,
					"message": check.Message,
				}
				if check.Hint != "" {
					row["hint"] = check.Hint
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\nSummary: ok=%t pass=%d warn=%d fail=%d skip=%d total=%d\n",
				payload.Summary.OK, payload.Summary.Pass, payload.Summary.Warn, payload.Summary.Fail, payload.Summary.Skip, payload.Summary.Total)
			if len(payload.NextSteps) > 0 {
				fmt.Fprintln(w, "\nNext steps:")
				for _, step := range payload.NextSteps {
					fmt.Fprintf(w, "- %s\n", step)
				}
			}
		})
		return nil
	},
}

// resolveBotTenantToken fetches a bot tenant access token via the credential
// provider using the caller-supplied context so that diagnose can apply a
// short, cancellable timeout without affecting the shortcut-wide ctx.
func resolveBotTenantToken(ctx context.Context, runtime *common.RuntimeContext) (string, error) {
	if runtime == nil || runtime.Factory == nil || runtime.Factory.Credential == nil {
		return "", fmt.Errorf("credential provider not initialized")
	}
	result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(core.AsBot, runtime.Config.AppID))
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return result.Token, nil
}

func diagnoseBotApp(runtime *common.RuntimeContext) botReceiveCheck {
	if runtime.Config == nil || strings.TrimSpace(runtime.Config.AppID) == "" {
		return failBotReceiveCheck(
			"app_resolved",
			"bot app configuration is unavailable",
			"run `lark-cli config init --new` to configure the app before diagnosing bot receive issues",
		)
	}
	return passBotReceiveCheck("app_resolved", fmt.Sprintf("app resolved: %s (%s)", runtime.Config.AppID, runtime.Config.Brand))
}

func summarizeBotReceiveChecks(checks []botReceiveCheck) botReceiveSummary {
	summary := botReceiveSummary{OK: true, Total: len(checks)}
	for _, check := range checks {
		switch check.Status {
		case "pass":
			summary.Pass++
		case "warn":
			summary.Warn++
		case "fail":
			summary.Fail++
			summary.OK = false
		case "skip":
			summary.Skip++
		}
	}
	return summary
}

func collectBotReceiveNextSteps(checks []botReceiveCheck) []string {
	seen := make(map[string]struct{})
	var steps []string
	for _, check := range checks {
		if check.Hint == "" || (check.Status != "fail" && check.Status != "warn") {
			continue
		}
		if _, ok := seen[check.Hint]; ok {
			continue
		}
		seen[check.Hint] = struct{}{}
		steps = append(steps, check.Hint)
	}
	sort.Strings(steps)
	return steps
}

func diagnoseBotReceiveWebsocket(ctx context.Context, runtime *common.RuntimeContext, eventType string, timeout time.Duration) botReceiveCheck {
	if runtime.Config == nil || strings.TrimSpace(runtime.Config.AppID) == "" || strings.TrimSpace(runtime.Config.AppSecret) == "" {
		return skipBotReceiveCheck("endpoint_ws", "skipped because app configuration is unavailable")
	}

	domain := lark.FeishuBaseUrl
	if runtime.Config.Brand == core.BrandLark {
		domain = lark.LarkBaseUrl
	}

	sdkLogger := &silentSDKLogger{}
	eventDispatcher := dispatcher.NewEventDispatcher("", "")
	eventDispatcher.InitConfig(larkevent.WithLogger(sdkLogger))
	eventDispatcher.OnCustomizedEvent(eventType, func(context.Context, *larkevent.EventReq) error { return nil })

	// wsCtx is deliberately derived from ctx so that Ctrl-C propagates, and
	// a separate readiness timer bounds how long we wait for Start to
	// surface an error. We cannot claim readiness from the timer firing
	// alone — see the `case <-readinessTimer.C` branch below — but we
	// distinguish our own timeout from a parent-ctx cancel in the result.
	wsCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cli := larkws.NewClient(runtime.Config.AppID, runtime.Config.AppSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithDomain(domain),
		larkws.WithLogger(sdkLogger),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cli.Start(wsCtx)
	}()

	readinessTimer := time.NewTimer(timeout)
	defer readinessTimer.Stop()

	select {
	case err := <-errCh:
		if err == nil {
			// Forward-compat: the current SDK's Start blocks on select{}
			// on the happy path and never returns nil, so this branch is
			// not reachable today. If a future SDK returns nil on clean
			// shutdown, we still cannot infer readiness from it alone —
			// warn rather than claim a false-positive pass.
			return warnBotReceiveCheck(
				"endpoint_ws",
				fmt.Sprintf("event WebSocket for %s shut down cleanly, but connection readiness was not confirmed", eventType),
				"retry to re-run the probe, or verify long-lived WebSocket connectivity and event subscription configuration",
			)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return warnBotReceiveCheck(
				"endpoint_ws",
				fmt.Sprintf("event WebSocket probe for %s was canceled before readiness could be confirmed", eventType),
				"retry without interruption, or raise --timeout if the network/proxy is slow",
			)
		}
		return failBotReceiveCheck(
			"endpoint_ws",
			fmt.Sprintf("event WebSocket startup failed for %s: %v", eventType, err),
			"check event subscription settings, bot receive permission, and network/proxy settings for long connections",
		)
	case <-readinessTimer.C:
		// The readiness timer fired before Start returned. Absent any
		// positive signal from the SDK ("connected and subscribed"), we
		// CANNOT claim readiness here: Start blocks on select{} in the
		// happy path, but it also blocks while the SDK's internal
		// autoReconnect loop retries transient connect failures, and the
		// SDK does not surface those retries to the caller. Report as
		// warn with guidance instead of a false-positive pass.
		return warnBotReceiveCheck(
			"endpoint_ws",
			fmt.Sprintf("event WebSocket for %s did not report a startup error within %s, but connection readiness was not confirmed", eventType, timeout),
			"retry with a larger --timeout, or verify long-lived WebSocket connectivity, proxy settings, and event subscription configuration",
		)
	case <-ctx.Done():
		return warnBotReceiveCheck(
			"endpoint_ws",
			fmt.Sprintf("event WebSocket probe for %s was canceled before readiness could be confirmed", eventType),
			"retry without interruption, or raise --timeout if the network/proxy is slow",
		)
	}
}

func passBotReceiveCheck(name, msg string) botReceiveCheck {
	return botReceiveCheck{Name: name, Status: "pass", Message: msg}
}

func warnBotReceiveCheck(name, msg, hint string) botReceiveCheck {
	return botReceiveCheck{Name: name, Status: "warn", Message: msg, Hint: hint}
}

func failBotReceiveCheck(name, msg, hint string) botReceiveCheck {
	return botReceiveCheck{Name: name, Status: "fail", Message: msg, Hint: hint}
}

func skipBotReceiveCheck(name, msg string) botReceiveCheck {
	return botReceiveCheck{Name: name, Status: "skip", Message: msg}
}

// deriveScopeAdvisoryCheck maps a concrete event type to the scope name it
// most commonly requires. The CLI cannot confirm a bot's actual granted
// scopes (tenant access tokens do not carry that metadata), so this is
// always an advisory — `warn` when we can name a scope, and a softer `warn`
// pointing the user at the developer console when we cannot.
func deriveScopeAdvisoryCheck(eventType string) botReceiveCheck {
	scope, ok := deriveScopeForEventType(eventType)
	if !ok {
		return warnBotReceiveCheck(
			"scope_required",
			fmt.Sprintf("CLI cannot derive the required scope for event `%s`", eventType),
			fmt.Sprintf("look up the required scope for `%s` in the developer console and confirm the app has it enabled and approved", eventType),
		)
	}
	return warnBotReceiveCheck(
		"scope_required",
		fmt.Sprintf("event `%s` typically requires scope `%s`; the CLI cannot verify it for bot identity", eventType, scope),
		fmt.Sprintf("confirm the app has `%s` enabled and approved in the developer console", scope),
	)
}

// deriveScopeForEventType returns the canonical scope for a known event type,
// or (_, false) if the CLI does not know a mapping. The table is intentionally
// narrow — we only encode scope names we are confident about; anything else
// falls through to a generic hint.
func deriveScopeForEventType(eventType string) (string, bool) {
	switch {
	case eventType == "im.message.receive_v1":
		return "im:message:receive_as_bot", true
	case eventType == "im.message.message_read_v1":
		return "im:message", true
	case strings.HasPrefix(eventType, "im.chat.member."):
		return "im:chat:readonly", true
	}
	return "", false
}
