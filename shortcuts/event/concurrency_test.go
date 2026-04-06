// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func makeInboundEnvelopeWithEventID(eventID, eventType, eventJSON string) InboundEnvelope {
	body := fmt.Sprintf(`{"schema":"2.0","header":{"event_id":"%s","event_type":"%s"},"event":%s}`, eventID, eventType, eventJSON)
	return InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: nowForTest(),
		RawPayload: []byte(body),
	}
}

func nowForTest() time.Time {
	return time.Unix(1700000000, 0).UTC()
}

func TestOutputRouterWriteRecordConcurrent(t *testing.T) {
	router := &outputRouter{
		defaultDir: filepath.Join(t.TempDir(), "events"),
		seq:        new(uint64),
		writers:    map[string]*dirRecordWriter{},
	}

	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			if err := router.WriteRecord("im.message.receive_v1", map[string]interface{}{
				"event_type": "im.message.receive_v1",
				"event_id":   fmt.Sprintf("evt-%03d", i),
			}); err != nil {
				t.Errorf("WriteRecord() error = %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestPipelineConcurrentProcessCountsAllDispatches(t *testing.T) {
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{
		id:        "counting-handler",
		eventType: "im.message.receive_v1",
		fn: func(_ context.Context, evt *Event) HandlerResult {
			return HandlerResult{Status: HandlerStatusHandled, Output: map[string]interface{}{"event_id": evt.EventID}}
		},
	}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}

	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformCompact}, io.Discard, io.Discard)

	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			p.Process(context.Background(), makeInboundEnvelopeWithEventID(
				fmt.Sprintf("evt-%03d", i),
				"im.message.receive_v1",
				fmt.Sprintf(`{"message":{"message_id":"om_%03d"}}`, i),
			))
		}(i)
	}
	wg.Wait()

	if got, want := p.EventCount(), int64(workers); got != want {
		t.Fatalf("EventCount() = %d, want %d", got, want)
	}
}
