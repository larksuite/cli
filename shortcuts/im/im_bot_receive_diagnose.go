// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package im

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

const defaultIMReceiveEventType = "im.message.receive_v1"

var (
	botReceiveProbeEndpoint  = probeBotReceiveEndpoint
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

var ImBotReceiveDiagnose = common.Shortcut{
	Service:     "im",
	Command:     "+bot-receive-diagnose",
	Description: "Diagnose why a bot does not receive IM message events (bot-only, read-only)",
	Risk:        "read",
	Scopes:      []string{"im:message:receive_as_bot"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "offline", Type: "bool", Desc: "skip network and WebSocket checks; only verify local bot configuration and token readiness"},
		{Name: "timeout", Type: "int", Default: "5", Desc: "timeout in seconds for network and WebSocket checks (1-60)"},
		{Name: "event-type", Default: defaultIMReceiveEventType, Desc: "event type to diagnose (default im.message.receive_v1)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Diagnose IM bot receive readiness without changing remote state").
			Set("command", "im +bot-receive-diagnose").
			Set("event_type", runtime.Str("event-type")).
			Set("offline", runtime.Bool("offline")).
			Set("timeout_sec", runtime.Int("timeout"))
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
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

		if strings.TrimSpace(runtime.Config.AppSecret) == "" {
			checks = append(checks, failBotReceiveCheck(
				"app_credential",
				"app secret is empty",
				"run `lark-cli config init --new` or update the configured app secret before diagnosing bot receive events",
			))
		} else {
			checks = append(checks, passBotReceiveCheck("app_credential", "app credentials are configured"))
		}

		token, err := runtime.AccessToken()
		if err != nil {
			checks = append(checks, failBotReceiveCheck(
				"token_bot",
				fmt.Sprintf("failed to get bot tenant access token: %v", err),
				"check app id/app secret, app status, and bot-related permissions in the developer console",
			))
		} else if strings.TrimSpace(token) == "" {
			checks = append(checks, failBotReceiveCheck(
				"token_bot",
				"tenant access token is empty",
				"check app id/app secret and retry",
			))
		} else {
			checks = append(checks, passBotReceiveCheck("token_bot", "bot tenant access token acquired successfully"))
		}

		if offline {
			checks = append(checks,
				skipBotReceiveCheck("endpoint_open", "skipped (--offline)"),
				skipBotReceiveCheck("endpoint_ws", "skipped (--offline)"),
			)
		} else {
			openURL := core.ResolveEndpoints(runtime.Config.Brand).Open
			if err := botReceiveProbeEndpoint(ctx, runtime, openURL, timeout); err != nil {
				checks = append(checks, failBotReceiveCheck(
					"endpoint_open",
					fmt.Sprintf("%s unreachable: %v", openURL, err),
					"check network, proxy, or firewall settings before diagnosing IM event delivery",
				))
			} else {
				checks = append(checks, passBotReceiveCheck("endpoint_open", openURL+" reachable"))
			}

			if strings.TrimSpace(token) == "" {
				checks = append(checks, skipBotReceiveCheck("endpoint_ws", "skipped because bot token is unavailable"))
			} else {
				checks = append(checks, botReceiveProbeWebsocket(ctx, runtime, eventType, timeout))
			}
		}

		checks = append(checks,
			warnBotReceiveCheck(
				"event_subscription",
				fmt.Sprintf("CLI cannot verify whether %s is subscribed in the developer console", eventType),
				fmt.Sprintf("verify that event `%s` is added in the app's event subscriptions", eventType),
			),
			warnBotReceiveCheck(
				"receive_scope",
				"message receive permission cannot be fully pre-verified for bot identity",
				"confirm the app has `im:message:receive_as_bot` enabled and approved in the developer console",
			),
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

func diagnoseBotApp(runtime *common.RuntimeContext) botReceiveCheck {
	if runtime.Config == nil || strings.TrimSpace(runtime.Config.AppID) == "" {
		return failBotReceiveCheck("app_resolved", "bot app configuration is unavailable", "run `lark-cli config init --new` to configure the app before diagnosing IM bot receive issues")
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

func probeBotReceiveEndpoint(ctx context.Context, runtime *common.RuntimeContext, url string, timeout time.Duration) error {
	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		httpClient = &http.Client{}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func diagnoseBotReceiveWebsocket(ctx context.Context, runtime *common.RuntimeContext, eventType string, timeout time.Duration) botReceiveCheck {
	domain := lark.FeishuBaseUrl
	if runtime.Config.Brand == core.BrandLark {
		domain = lark.LarkBaseUrl
	}

	sdkLogger := &silentSDKLogger{}
	eventDispatcher := dispatcher.NewEventDispatcher("", "")
	eventDispatcher.InitConfig(larkevent.WithLogger(sdkLogger))
	eventDispatcher.OnCustomizedEvent(eventType, func(context.Context, *larkevent.EventReq) error { return nil })

	wsCtx, cancel := context.WithTimeout(ctx, timeout)
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

	select {
	case err := <-errCh:
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return passBotReceiveCheck("endpoint_ws", fmt.Sprintf("event WebSocket for %s did not fail within %s", eventType, timeout))
		}
		return failBotReceiveCheck(
			"endpoint_ws",
			fmt.Sprintf("event WebSocket startup failed for %s: %v", eventType, err),
			"check event subscription settings, bot receive permission, and network/proxy settings for long connections",
		)
	case <-wsCtx.Done():
		return passBotReceiveCheck("endpoint_ws", fmt.Sprintf("event WebSocket for %s did not fail within %s", eventType, timeout))
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
