// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package source

import (
	"context"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

type mockSource struct {
	name   string
	events []*event.RawEvent
}

func (s *mockSource) Name() string { return s.name }
func (s *mockSource) Start(ctx context.Context, _ []string, emit func(*event.RawEvent), _ StatusNotifier) error {
	for _, e := range s.events {
		emit(e)
	}
	<-ctx.Done()
	return nil
}

func TestRegister(t *testing.T) {
	ResetForTest()

	src := &mockSource{name: "test-source"}
	Register(src)

	sources := All()
	if len(sources) != 1 || sources[0].Name() != "test-source" {
		t.Errorf("unexpected sources: %v", sources)
	}
}

func TestMockSource_EmitsEvents(t *testing.T) {
	src := &mockSource{
		name: "test",
		events: []*event.RawEvent{
			{EventID: "1", EventType: "im.message.receive_v1"},
			{EventID: "2", EventType: "im.message.receive_v1"},
		},
	}

	received := make(chan *event.RawEvent, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go src.Start(ctx, nil, func(e *event.RawEvent) {
		received <- e
	}, nil)

	time.Sleep(50 * time.Millisecond)
	if len(received) != 2 {
		t.Errorf("expected 2 events, got %d", len(received))
	}
}
