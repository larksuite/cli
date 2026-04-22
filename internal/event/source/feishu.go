// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package source

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// maxEventBodyBytes caps an individual inbound event payload. Feishu's
// own upstream events are well under 100 KB in practice; 1 MB is a
// defensive headroom. Oversized payloads are dropped with a log line
// rather than fanned out — otherwise each subscriber's sendCh (cap 100)
// could hold 100× the oversized payload in memory.
const maxEventBodyBytes = 1 << 20

// FeishuSource connects to the Feishu WebSocket gateway and emits events.
type FeishuSource struct {
	AppID     string
	AppSecret string
	Domain    string
	Logger    *log.Logger
}

func (s *FeishuSource) Name() string { return "feishu-websocket" }

func (s *FeishuSource) Start(ctx context.Context, eventTypes []string, emit func(*event.RawEvent), notify StatusNotifier) error {
	d := dispatcher.NewEventDispatcher("", "")

	rawHandler := s.buildRawHandler(emit)

	for _, et := range eventTypes {
		d.OnCustomizedEvent(et, rawHandler)
	}

	opts := []larkws.ClientOption{larkws.WithEventHandler(d)}
	if s.Domain != "" {
		opts = append(opts, larkws.WithDomain(s.Domain))
	}
	if s.Logger != nil || notify != nil {
		opts = append(opts, larkws.WithLogLevel(larkcore.LogLevelInfo))
		opts = append(opts, larkws.WithLogger(&sdkLogger{l: s.Logger, notify: notify}))
	}

	if notify != nil {
		notify(protocol.SourceStateConnecting, "")
	}
	cli := larkws.NewClient(s.AppID, s.AppSecret, opts...)

	errCh := make(chan error, 1)
	go func() { errCh <- cli.Start(ctx) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// buildRawHandler returns the per-event callback passed to the SDK
// dispatcher. Extracted from Start so unit tests can exercise the
// three failure modes (nil body, malformed JSON, missing header
// fields) without spinning up a WebSocket client. A nil Logger is
// tolerated — callers constructed with minimal test setup won't panic.
func (s *FeishuSource) buildRawHandler(emit func(*event.RawEvent)) func(context.Context, *larkevent.EventReq) error {
	return func(_ context.Context, e *larkevent.EventReq) error {
		if e.Body == nil {
			return nil
		}
		if len(e.Body) > maxEventBodyBytes {
			if s.Logger != nil {
				s.Logger.Printf("[feishu] drop oversized event: %d bytes > cap %d", len(e.Body), maxEventBodyBytes)
			}
			return nil
		}
		var envelope struct {
			Header struct {
				EventID    string `json:"event_id"`
				EventType  string `json:"event_type"`
				CreateTime string `json:"create_time"`
			} `json:"header"`
		}
		if err := json.Unmarshal(e.Body, &envelope); err != nil {
			if s.Logger != nil {
				preview := string(e.Body)
				if len(preview) > 200 {
					preview = preview[:200] + "...(truncated)"
				}
				s.Logger.Printf("[feishu] drop malformed event: unmarshal error: %v body=%s", err, preview)
			}
			return nil
		}
		if envelope.Header.EventID == "" || envelope.Header.EventType == "" {
			if s.Logger != nil {
				s.Logger.Printf("[feishu] drop event missing header fields: event_id=%q event_type=%q",
					envelope.Header.EventID, envelope.Header.EventType)
			}
			return nil
		}
		emit(&event.RawEvent{
			EventID:    envelope.Header.EventID,
			EventType:  envelope.Header.EventType,
			SourceTime: envelope.Header.CreateTime,
			Payload:    json.RawMessage(e.Body),
			Timestamp:  time.Now(),
		})
		return nil
	}
}

// sdkLogger adapts *log.Logger to larkcore.Logger. Every SDK line goes
// to bus.log; lines matching known lifecycle patterns also fire notify
// so consumers see WebSocket connect/disconnect/reconnect in real time.
type sdkLogger struct {
	l      *log.Logger
	notify StatusNotifier
}

func (a *sdkLogger) Debug(_ context.Context, _ ...interface{}) {}
func (a *sdkLogger) Info(_ context.Context, args ...interface{}) {
	msg := fmt.Sprint(args...)
	if a.l != nil {
		a.l.Output(2, "[SDK] "+msg)
	}
	a.tryNotify(msg, "")
}
func (a *sdkLogger) Warn(_ context.Context, args ...interface{}) {
	msg := fmt.Sprint(args...)
	if a.l != nil {
		a.l.Output(2, "[SDK WARN] "+msg)
	}
	a.tryNotify(msg, "")
}
func (a *sdkLogger) Error(_ context.Context, args ...interface{}) {
	msg := fmt.Sprint(args...)
	if a.l != nil {
		a.l.Output(2, "[SDK ERROR] "+msg)
	}
	// Errors usually manifest as disconnects; surface them with the error
	// text as detail so users see why.
	a.tryNotify(msg, msg)
}

// reconnectAttemptRe captures the attempt number from SDK's "trying to
// reconnect: N" lines so we can surface it as detail.
var reconnectAttemptRe = regexp.MustCompile(`reconnect:?\s*(\d+)`)

// tryNotify classifies an SDK log line into a lifecycle state. A nil
// notify or a line we don't recognise is a no-op — non-lifecycle noise
// (e.g. heartbeat) stays in bus.log but doesn't reach the user.
//
// Matching is HasPrefix, not Contains: SDK log lines are of the form
// "<verb> to <url>[conn_id=...]", so "connected to " and "disconnected
// to " are mutually exclusive at the start of the string. A prior
// Contains-based switch accidentally matched "connected to ws" inside
// "disconnected to wss://...", misreporting every disconnect as a
// reconnect. See TestTryNotify_Classify for the regression case.
func (a *sdkLogger) tryNotify(msg, errDetail string) {
	if a.notify == nil {
		return
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.HasPrefix(lower, sdkLogReconnecting):
		detail := ""
		if m := reconnectAttemptRe.FindStringSubmatch(lower); len(m) == 2 {
			detail = "attempt " + m[1]
		}
		a.notify(protocol.SourceStateReconnecting, detail)
	case strings.HasPrefix(lower, sdkLogDisconnected):
		a.notify(protocol.SourceStateDisconnected, errDetail)
	case strings.HasPrefix(lower, sdkLogConnected):
		a.notify(protocol.SourceStateConnected, "")
	}
}

var _ larkcore.Logger = (*sdkLogger)(nil)
