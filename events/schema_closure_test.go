// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	event "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/processing"

	_ "github.com/larksuite/cli/events"
)

// closureAPIClient answers any API call with a benign error: a handler facing
// a malformed payload must decide to drop before it ever needs the API.
type closureAPIClient struct{}

func (closureAPIClient) CallAPI(context.Context, string, string, any) (json.RawMessage, error) {
	return nil, errors.New("no API access for malformed input")
}

// Every Processed EventKey declares an output schema; its stdout must stay
// inside that schema. A payload that cannot be decoded therefore has exactly
// one legal outcome: a malformed drop. Passing the raw envelope through would
// hand consumers a shape the schema never described.
//
// Native keys (Process == nil) are exempt by contract: their declared output
// is the raw envelope itself.
func TestAllKeys_MalformedPayloadStaysSchemaClosed(t *testing.T) {
	const wantProcessedKeys = 13

	processed := 0
	for _, def := range event.ListAll() {
		if def.Process == nil {
			continue
		}
		processed++
		out, err := safeProcess(t, def, json.RawMessage(`this is definitely not valid json {{{`))
		if out != nil {
			t.Errorf("%s: malformed payload produced stdout output; it must be dropped", def.Key)
		}
		if !processing.IsDropMalformed(err) {
			t.Errorf("%s: malformed payload must be dropped with a malformed marker, got err=%v", def.Key, err)
		}
	}
	if processed == 0 {
		t.Fatal("no processed keys were exercised; the gate scanned nothing")
	}
	if processed != wantProcessedKeys {
		t.Fatalf("exercised %d processed keys, want exactly %d; update the count when keys are deliberately added or removed", processed, wantProcessedKeys)
	}
}

// safeProcess isolates a panicking handler to a per-key finding instead of
// aborting the whole gate: a handler that dereferences before decoding is a
// bug in that key, not a reason to stop scanning the rest.
func safeProcess(t *testing.T, def *event.KeyDefinition, payload json.RawMessage) (out json.RawMessage, err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: Process panicked on malformed payload: %v", def.Key, r)
			out, err = nil, nil
		}
	}()
	raw := &event.RawEvent{
		EventID:   "evt-closure-1",
		EventType: def.EventType,
		Payload:   payload,
		Timestamp: time.Unix(0, 0),
	}
	return def.Process(context.Background(), closureAPIClient{}, raw, map[string]string{})
}
