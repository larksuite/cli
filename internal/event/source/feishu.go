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

const maxEventBodyBytes = 1 << 20 // bound per-subscriber sendCh memory under runaway payloads

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

// buildRawHandler is extracted from Start so unit tests can exercise it without a WS client.
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

// sdkLogger forwards every SDK line to bus.log; lifecycle lines also fire notify.
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
	// Errors usually manifest as disconnects; pass msg as detail.
	a.tryNotify(msg, msg)
}

var reconnectAttemptRe = regexp.MustCompile(`reconnect:?\s*(\d+)`)

// tryNotify uses HasPrefix (not Contains): "connected to" matches inside "disconnected to" otherwise.
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
