// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// stderrLogger redirects SDK log output to an io.Writer (stderr),
// preventing SDK logs from polluting the stdout data stream.
// Debug logs are always suppressed to avoid noisy event-loop output.
// When quiet is true, Info logs are also suppressed; Warn and Error always print.
type stderrLogger struct {
	w     io.Writer
	quiet bool
}

func (l *stderrLogger) Debug(_ context.Context, _ ...interface{}) {}
func (l *stderrLogger) Info(_ context.Context, args ...interface{}) {
	if !l.quiet {
		fmt.Fprintln(l.w, append([]interface{}{"[SDK Info]"}, args...)...)
	}
}
func (l *stderrLogger) Warn(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.w, append([]interface{}{"[SDK Warn]"}, args...)...)
}
func (l *stderrLogger) Error(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.w, append([]interface{}{"[SDK Error]"}, args...)...)
}

var _ larkcore.Logger = (*stderrLogger)(nil)

// subscribedEventTypes are the event types wired into the SDK dispatcher.
// Keep this list aligned with NewBuiltinHandlerRegistry because the current SDK
// only delivers websocket events to explicitly registered event types here.
var subscribedEventTypes = []string{
	"im.message.receive_v1",
	"im.message.message_read_v1",
	"im.message.reaction.created_v1",
	"im.message.reaction.deleted_v1",
	"im.chat.member.bot.added_v1",
	"im.chat.member.bot.deleted_v1",
	"im.chat.member.user.added_v1",
	"im.chat.member.user.withdrawn_v1",
	"im.chat.member.user.deleted_v1",
	"im.chat.updated_v1",
	"im.chat.disbanded_v1",
}

func subscribedEventTypesFor(eventTypesStr string) []string {
	filter := NewEventTypeFilter(eventTypesStr)
	if filter == nil {
		return nil
	}
	return filter.Types()
}

var EventSubscribe = common.Shortcut{
	Service:     "event",
	Command:     "+subscribe",
	Description: "Subscribe to Lark events via WebSocket (NDJSON output)",
	Risk:        "read",
	Scopes:      []string{}, // no direct OAPI; scopes depend on subscribed event types
	AuthTypes:   []string{"bot"},
	Flags: []common.Flag{
		// Output destination — where events go
		{Name: "output-dir", Desc: "write each event as a JSON file in this directory (default: stdout)"},
		{Name: "route", Type: "string_array", Desc: "regex-based event routing (e.g. --route '^im\\.message=dir:./im/' --route '^contact\\.=dir:./contacts/'); unmatched events fall through to --output-dir or stdout"},
		// Output format — how events are serialized
		{Name: "compact", Type: "bool", Desc: "flat key-value output: extract text, strip noise fields"},
		{Name: "json", Type: "bool", Desc: "pretty-print JSON instead of NDJSON"},
		// Filtering — which events reach the pipeline
		{Name: "event-types", Desc: "comma-separated event types to subscribe; only use when you do not need other events (omit for catch-all)"},
		{Name: "filter", Desc: "regex to further filter events by event_type"},
		// Behavior
		{Name: "quiet", Type: "bool", Desc: "suppress stderr status messages"},
		{Name: "force", Type: "bool", Desc: "bypass single-instance lock (UNSAFE: server randomly splits events across connections, each instance only receives a subset)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		eventTypesDisplay := "(catch-all)"
		if s := runtime.Str("event-types"); s != "" {
			eventTypesDisplay = s
		}
		filterDisplay := "(none)"
		if s := runtime.Str("filter"); s != "" {
			filterDisplay = s
		}
		outputDirDisplay := "(stdout)"
		if s := runtime.Str("output-dir"); s != "" {
			outputDirDisplay = s
		}
		routeDisplay := "(none)"
		if routes := runtime.StrArray("route"); len(routes) > 0 {
			routeDisplay = strings.Join(routes, "; ")
		}
		return common.NewDryRunAPI().
			Desc("Subscribe to Lark events via WebSocket (long-running)").
			Set("command", "event +subscribe").
			Set("app_id", runtime.Config.AppID).
			Set("event_types", eventTypesDisplay).
			Set("filter", filterDisplay).Set("output_dir", outputDirDisplay).
			Set("route", routeDisplay)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		eventTypesStr := runtime.Str("event-types")
		filterStr := runtime.Str("filter")
		jsonFlag := runtime.Bool("json")
		compactFlag := runtime.Bool("compact")
		outputDir := runtime.Str("output-dir")
		quietFlag := runtime.Bool("quiet")
		routeSpecs := runtime.StrArray("route")
		forceFlag := runtime.Bool("force")

		// Validate output directory path before any work
		if outputDir != "" {
			safePath, err := validate.SafeOutputPath(outputDir)
			if err != nil {
				return output.ErrValidation("unsafe output path: %s", err)
			}
			outputDir = safePath
		}

		errOut := runtime.IO().ErrOut
		out := runtime.IO().Out

		info := func(msg string) {
			if !quietFlag {
				fmt.Fprintln(errOut, msg)
			}
		}

		// --- Single-instance lock ---
		if !forceFlag {
			lock, err := lockfile.ForSubscribe(runtime.Config.AppID)
			if err != nil {
				return fmt.Errorf("failed to create lock: %w", err)
			}
			if err := lock.TryLock(); err != nil {
				return output.ErrValidation(
					"another event +subscribe instance is already running for app %s\n"+
						"  Only one subscriber per app is allowed to prevent competing consumers.\n"+
						"  Use --force to bypass this check.",
					runtime.Config.AppID,
				)
			}
			defer lock.Unlock()
		}

		// --- Build filter chain ---
		explicitTypes := subscribedEventTypesFor(eventTypesStr)
		eventTypeFilter := NewEventTypeFilter(eventTypesStr)
		regexFilter, err := NewRegexFilter(filterStr)
		if err != nil {
			return output.ErrValidation("invalid --filter regex: %s", filterStr)
		}
		var filterList []EventFilter
		if eventTypeFilter != nil {
			filterList = append(filterList, eventTypeFilter)
		}
		if regexFilter != nil {
			filterList = append(filterList, regexFilter)
		}
		filters := NewFilterChain(filterList...)

		_ = jsonFlag
		hasRoutes := len(routeSpecs) > 0
		var recordWriter OutputRecordWriter
		if hasRoutes || outputDir != "" {
			router, err := ParseRoutes(routeSpecs)
			if err != nil {
				return output.ErrValidation("invalid --route: %v", err)
			}
			recordWriter = &outputRouter{
				router:     router,
				defaultDir: outputDir,
				fallback:   ndjsonRecordWriter{w: out},
				seq:        new(uint64),
				writers:    map[string]*dirRecordWriter{},
			}
		}

		// --- Build pipeline ---
		mode := TransformRaw
		if compactFlag {
			mode = TransformCompact
		}
		pipeline := newEventPipeline(NewBuiltinHandlerRegistry(), filters, PipelineConfig{
			Mode:       mode,
			Quiet:      quietFlag,
			PrettyJSON: jsonFlag,
		}, out, errOut, recordWriter)

		// --- Build SDK event dispatcher ---
		rawHandler := func(ctx context.Context, event *larkevent.EventReq) error {
			if event.Body == nil {
				return nil
			}
			pipeline.Process(ctx, BuildWebSocketEnvelope(event.Body))
			return nil
		}

		sdkLogger := &stderrLogger{w: errOut, quiet: quietFlag}

		eventDispatcher := dispatcher.NewEventDispatcher("", "")
		eventDispatcher.InitConfig(larkevent.WithLogger(sdkLogger))
		if explicitTypes != nil {
			for _, et := range explicitTypes {
				eventDispatcher.OnCustomizedEvent(et, rawHandler)
			}
		} else {
			for _, et := range subscribedEventTypes {
				eventDispatcher.OnCustomizedEvent(et, rawHandler)
			}
		}

		// --- WebSocket ---
		domain := lark.FeishuBaseUrl
		if runtime.Config.Brand == core.BrandLark {
			domain = lark.LarkBaseUrl
		}

		info(fmt.Sprintf("%sConnecting to Lark event WebSocket...%s", output.Cyan, output.Reset))
		if explicitTypes != nil {
			info(fmt.Sprintf("Listening for: %s%s%s", output.Green, strings.Join(explicitTypes, ", "), output.Reset))
		} else {
			info(fmt.Sprintf("Listening in %scatch-all%s mode for all delivered event types supported by the current SDK dispatcher", output.Green, output.Reset))
			info(fmt.Sprintf("%sRegistration:%s SDK default handler via OnCustomizedEvent(\"\", ...)", output.Dim, output.Reset))
			info(fmt.Sprintf("%sTip:%s use --event-types to narrow subscription", output.Dim, output.Reset))
		}
		if regexFilter != nil {
			info(fmt.Sprintf("Filter: %s%s%s", output.Yellow, regexFilter.String(), output.Reset))
		}
		if hasRoutes {
			for _, spec := range routeSpecs {
				info(fmt.Sprintf("  Route: %s%s%s", output.Green, spec, output.Reset))
			}
		}

		cli := larkws.NewClient(runtime.Config.AppID, runtime.Config.AppSecret,
			larkws.WithEventHandler(eventDispatcher),
			larkws.WithDomain(domain),
			larkws.WithLogger(sdkLogger),
		)

		// --- Graceful shutdown ---
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		startErrCh := make(chan error, 1)
		go func() {
			startErrCh <- cli.Start(ctx)
		}()

		info(fmt.Sprintf("%s%sConnected.%s Waiting for events... (Ctrl+C to stop)", output.Bold, output.Green, output.Reset))

		select {
		case sig, ok := <-sigCh:
			if ok && sig != nil {
				info(fmt.Sprintf("\n%sReceived %s, shutting down...%s (received %s%d%s events)", output.Yellow, sig, output.Reset, output.Bold, pipeline.EventCount(), output.Reset))
			}
			return nil
		case err, ok := <-startErrCh:
			if !ok {
				return nil
			}
			if err != nil {
				return output.ErrNetwork("WebSocket connection failed: %v", err)
			}
			return nil
		}
	},
}
