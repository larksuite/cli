// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

// helper to build a RawEvent from event-level JSON and header fields.
func makeRawEvent(eventType string, eventJSON string) *RawEvent {
	return &RawEvent{
		Schema: "2.0",
		Header: larkevent.EventHeader{
			EventType: eventType,
			EventID:   "ev_test",
		},
		Event: json.RawMessage(eventJSON),
	}
}

func makeInboundEnvelope(eventType, eventJSON string) InboundEnvelope {
	body := `{"schema":"2.0","header":{"event_id":"ev_test","event_type":"` + eventType + `"},"event":` + eventJSON + `}`
	return InboundEnvelope{
		Source:     SourceWebSocket,
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
		RawPayload: []byte(body),
	}
}

func TestBuildWebSocketEnvelope(t *testing.T) {
	body := []byte(`{"schema":"2.0","header":{"event_id":"ev_test","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123"}}}`)
	before := time.Now()
	env := BuildWebSocketEnvelope(body)
	after := time.Now()

	if env.Source != SourceWebSocket {
		t.Fatalf("Source = %q, want %q", env.Source, SourceWebSocket)
	}
	if env.ReceivedAt.Before(before) || env.ReceivedAt.After(after) {
		t.Fatalf("ReceivedAt = %v, want between %v and %v", env.ReceivedAt, before, after)
	}
	if !bytes.Equal(env.RawPayload, body) {
		t.Fatalf("RawPayload = %s, want %s", env.RawPayload, body)
	}
	if &env.RawPayload[0] == &body[0] {
		t.Fatal("RawPayload should copy input bytes")
	}
}

func TestBuildWebSocketEnvelopePreservesValidJSON(t *testing.T) {
	body := []byte(`{"schema":"2.0","header":{"event_id":"ev_test","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123"}}}`)
	env := BuildWebSocketEnvelope(body)

	var decoded map[string]interface{}
	if err := json.Unmarshal(env.RawPayload, &decoded); err != nil {
		t.Fatalf("RawPayload should remain valid JSON: %v", err)
	}
}

func makeTestRegistry(handler EventHandler, fallback EventHandler) *HandlerRegistry {
	registry := NewHandlerRegistry()
	if handler != nil {
		if handler.EventType() != "" {
			if err := registry.RegisterEventHandler(handler); err != nil {
				panic(err)
			}
		} else if handler.Domain() != "" {
			if err := registry.RegisterDomainHandler(handler); err != nil {
				panic(err)
			}
		}
	}
	if fallback != nil {
		if err := registry.SetFallbackHandler(fallback); err != nil {
			panic(err)
		}
	}
	return registry
}

// --- Registry ---

func TestRegistryLookup(t *testing.T) {
	r := DefaultRegistry()
	p := r.Lookup("im.message.receive_v1")
	if p.EventType() != "im.message.receive_v1" {
		t.Errorf("got %q", p.EventType())
	}
	p2 := r.Lookup("unknown.type")
	if p2.EventType() != "" {
		t.Errorf("fallback should have empty EventType, got %q", p2.EventType())
	}
}

func TestRegistryDuplicateReturnsError(t *testing.T) {
	r := NewProcessorRegistry(&GenericProcessor{})
	if err := r.Register(&ImMessageProcessor{}); err != nil {
		t.Fatalf("first register should succeed: %v", err)
	}
	if err := r.Register(&ImMessageProcessor{}); err == nil {
		t.Error("expected error on duplicate registration")
	}
}

// --- Filters ---

func TestEventTypeFilter(t *testing.T) {
	f := NewEventTypeFilter("im.message.receive_v1, drive.file.edit_v1")
	if !f.Allow("im.message.receive_v1") {
		t.Error("should allow")
	}
	if f.Allow("unknown.type") {
		t.Error("should reject")
	}
}

func TestEventTypeFilter_Empty(t *testing.T) {
	if f := NewEventTypeFilter(""); f != nil {
		t.Error("empty should return nil")
	}
}

func TestRegexFilter(t *testing.T) {
	f, err := NewRegexFilter("im\\.message\\..*")
	if err != nil {
		t.Fatal(err)
	}
	if !f.Allow("im.message.receive_v1") {
		t.Error("should match")
	}
	if f.Allow("drive.file.edit_v1") {
		t.Error("should not match")
	}
}

func TestRegexFilter_Invalid(t *testing.T) {
	_, err := NewRegexFilter("[invalid")
	if err == nil {
		t.Error("should error")
	}
}

func TestFilterChain(t *testing.T) {
	etf := NewEventTypeFilter("im.message.receive_v1, drive.file.edit_v1")
	rf, _ := NewRegexFilter("im\\..*")
	chain := NewFilterChain(etf, rf)

	if !chain.Allow("im.message.receive_v1") {
		t.Error("both filters pass, should allow")
	}
	if chain.Allow("drive.file.edit_v1") {
		t.Error("regex rejects drive, should block")
	}

	empty := NewFilterChain()
	if !empty.Allow("anything") {
		t.Error("empty chain should allow all")
	}

	var nilChain *FilterChain
	if !nilChain.Allow("anything") {
		t.Error("nil chain should allow all")
	}
}

func TestEventTypeFilter_TypesSorted(t *testing.T) {
	f := NewEventTypeFilter("z.type,a.type,m.type")
	got := f.Types()
	want := []string{"a.type", "m.type", "z.type"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Types() = %v, want %v", got, want)
	}
}

// --- Processors ---

func TestImMessageProcessor_Raw(t *testing.T) {
	p := &ImMessageProcessor{}
	eventJSON := `{"message":{"id":"1"}}`
	raw := makeRawEvent("im.message.receive_v1", eventJSON)
	result, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent)
	if !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
	if result.Header.EventType != "im.message.receive_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
	if result.Schema != "2.0" {
		t.Errorf("Schema = %v", result.Schema)
	}
}

func TestGenericProcessor_Compact(t *testing.T) {
	p := &GenericProcessor{}
	eventJSON := `{"file_token":"xxx"}`
	raw := makeRawEvent("drive.file.edit_v1", eventJSON)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map[string]interface{}")
	}
	if result["file_token"] != "xxx" {
		t.Error("file_token should be preserved")
	}
	if result["type"] != "drive.file.edit_v1" {
		t.Errorf("type = %v, want drive.file.edit_v1", result["type"])
	}
	if result["event_id"] != "ev_test" {
		t.Errorf("event_id = %v, want ev_test", result["event_id"])
	}
}

func TestGenericProcessor_Raw(t *testing.T) {
	p := &GenericProcessor{}
	eventJSON := `{"schema":"2.0"}`
	raw := makeRawEvent("drive.file.edit_v1", eventJSON)
	result, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent)
	if !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
	if result.Header.EventType != "drive.file.edit_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

// --- Pipeline ---

func TestPipeline_NormalizesAndDispatchesEventHandler(t *testing.T) {
	var calls []string
	handler := &testEventHandler{
		id:        "event-handler",
		eventType: "im.message.receive_v1",
		result:    HandlerResult{Status: HandlerStatusHandled},
		called:    &calls,
	}
	registry := makeTestRegistry(handler, nil)
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(registry, filters, PipelineConfig{Mode: TransformCompact}, &out, &errOut)

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"id":"1"}}`))

	if got, want := calls, []string{"event-handler"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d output lines, want 1", len(lines))
	}
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("invalid NDJSON line: %v", err)
	}
	if record["event_type"] != "im.message.receive_v1" {
		t.Fatalf("event_type = %v", record["event_type"])
	}
	if record["handler_id"] != "event-handler" {
		t.Fatalf("handler_id = %v", record["handler_id"])
	}
	if record["domain"] != "im" {
		t.Fatalf("domain = %v", record["domain"])
	}
}

func TestPipeline_Filtered(t *testing.T) {
	filters := NewFilterChain(NewEventTypeFilter("im.message.receive_v1"))
	var out, errOut bytes.Buffer
	p := NewEventPipeline(NewHandlerRegistry(), filters, PipelineConfig{}, &out, &errOut)

	p.Process(context.Background(), makeInboundEnvelope("drive.file.edit_v1", `{}`))

	if out.Len() != 0 {
		t.Error("filtered event should produce no output")
	}
}

func TestNewBuiltinHandlerRegistryRegistersBuiltins(t *testing.T) {
	registry := NewBuiltinHandlerRegistry()
	if got := registry.EventHandlers("im.message.receive_v1"); len(got) != 1 || got[0].ID() != imMessageHandlerID {
		t.Fatalf("EventHandlers(im.message.receive_v1) = %#v, want built-in IM handler", got)
	}
	if got := registry.FallbackHandler(); got == nil || got.ID() != genericHandlerID {
		t.Fatalf("FallbackHandler() = %#v, want built-in generic fallback", got)
	}
}

func TestPipeline_PreservesHandlerCompactOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{id: "compact", eventType: "im.message.receive_v1", fn: func(_ context.Context, evt *Event) HandlerResult {
		return HandlerResult{
			Status: HandlerStatusHandled,
			Output: map[string]interface{}{
				"type":       evt.EventType,
				"message_id": "om_123",
				"content":    "hello",
			},
		}
	}}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}
	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformCompact}, &out, &errOut)

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatalf("invalid NDJSON line: %v", err)
	}
	if record["handler_id"] != "compact" {
		t.Fatalf("handler_id = %v, want compact", record["handler_id"])
	}
	if record["type"] != "im.message.receive_v1" {
		t.Fatalf("type = %v, want im.message.receive_v1", record["type"])
	}
	if record["message_id"] != "om_123" {
		t.Fatalf("message_id = %v, want om_123", record["message_id"])
	}
	if record["content"] != "hello" {
		t.Fatalf("content = %v, want hello", record["content"])
	}
}

func TestPipeline_CompactModeWritesDispatchRecord(t *testing.T) {
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{
		id:        "compact-handler",
		eventType: "im.message.receive_v1",
		fn: func(_ context.Context, evt *Event) HandlerResult {
			return HandlerResult{
				Status: HandlerStatusHandled,
				Output: map[string]interface{}{
					"type":       evt.EventType,
					"message_id": "om_123",
				},
			}
		},
	}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}

	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformCompact}, &out, &errOut)
	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatalf("invalid compact-mode output: %v", err)
	}
	if record["handler_id"] != "compact-handler" {
		t.Fatalf("handler_id = %v", record["handler_id"])
	}
	if record["message_id"] != "om_123" {
		t.Fatalf("message_id = %v", record["message_id"])
	}
	if record["status"] != "handled" {
		t.Fatalf("status = %v", record["status"])
	}
}

func TestPipeline_EventCountTracksInboundEventsWithMultipleHandlers(t *testing.T) {
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{
		id:        "event-handler",
		eventType: "im.message.receive_v1",
		fn: func(_ context.Context, evt *Event) HandlerResult {
			return HandlerResult{
				Status: HandlerStatusHandled,
				Output: map[string]interface{}{"type": evt.EventType, "source": "event"},
			}
		},
	}); err != nil {
		t.Fatalf("RegisterEventHandler(event) error = %v", err)
	}
	if err := registry.RegisterDomainHandler(handlerFuncWith{
		id:     "domain-handler",
		domain: "im",
		fn: func(_ context.Context, evt *Event) HandlerResult {
			return HandlerResult{
				Status: HandlerStatusHandled,
				Output: map[string]interface{}{"type": evt.EventType, "source": "domain"},
			}
		},
	}); err != nil {
		t.Fatalf("RegisterDomainHandler(domain) error = %v", err)
	}

	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformCompact}, &out, &errOut)
	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2 dispatch records", len(lines))
	}
	if got, want := p.EventCount(), int64(1); got != want {
		t.Fatalf("EventCount() = %d, want %d inbound event", got, want)
	}
	if got, want := p.DispatchCount(), int64(2); got != want {
		t.Fatalf("DispatchCount() = %d, want %d dispatch records", got, want)
	}
}

func TestPipeline_RawModeWritesEventRecord(t *testing.T) {
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{id: genericHandlerID, eventType: "im.message.receive_v1", fn: func(_ context.Context, evt *Event) HandlerResult {
		return HandlerResult{
			Status: HandlerStatusHandled,
			Output: map[string]interface{}{
				"content": "handler-output-should-not-leak",
			},
		}
	}}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}
	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformRaw}, &out, &errOut)
	rawPayload := `{"message":{"message_id":"om_123","message_type":"text","content":"{\"text\":\"hello\"}"}}`

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", rawPayload))

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatalf("invalid NDJSON line: %v", err)
	}
	if got, want := record["event_type"], "im.message.receive_v1"; got != want {
		t.Fatalf("event_type = %v, want %v", got, want)
	}
	if got, want := record["status"], string(HandlerStatusHandled); got != want {
		t.Fatalf("status = %v, want %v", got, want)
	}
	if got, want := record["source"], string(SourceWebSocket); got != want {
		t.Fatalf("source = %v, want %v", got, want)
	}
	if got := record["idempotency_key"]; got == nil || got == "" {
		t.Fatalf("idempotency_key = %v, want non-empty", got)
	}
	if got, want := record["raw_payload"], `{"schema":"2.0","header":{"event_id":"ev_test","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123","message_type":"text","content":"{\"text\":\"hello\"}"}}}`; got != want {
		t.Fatalf("raw_payload = %v, want %v", got, want)
	}
	payload, ok := record["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload = %T, want object", record["payload"])
	}
	message, ok := payload["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload.message = %T, want object", payload["message"])
	}
	if got, want := message["message_id"], "om_123"; got != want {
		t.Fatalf("payload.message.message_id = %v, want %v", got, want)
	}
	if got, want := message["content"], `{"text":"hello"}`; got != want {
		t.Fatalf("payload.message.content = %v, want %v", got, want)
	}
	if got, want := record["domain"], "im"; got != want {
		t.Fatalf("domain = %v, want %v", got, want)
	}
	if got, want := record["event_id"], "ev_test"; got != want {
		t.Fatalf("event_id = %v, want %v", got, want)
	}
	if _, exists := record["handler_id"]; exists {
		t.Fatalf("handler_id should be absent in raw mode: %v", record["handler_id"])
	}
	if _, exists := record["content"]; exists {
		t.Fatalf("content should be absent in raw mode: %v", record["content"])
	}
}

func TestPipeline_RawModeCanWriteViaOutputRouter(t *testing.T) {
	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	router, err := ParseRoutes([]string{"^im\\.message=dir:./im"})
	if err != nil {
		t.Fatalf("ParseRoutes() error = %v", err)
	}
	fallbackDir := filepath.Join(tmpDir, "default")
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{id: genericHandlerID, eventType: "im.message.receive_v1", fn: func(_ context.Context, evt *Event) HandlerResult {
		return HandlerResult{Status: HandlerStatusHandled}
	}}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}
	p := newEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformRaw}, &out, &errOut, &outputRouter{router: router, defaultDir: fallbackDir, fallback: ndjsonRecordWriter{w: &out}, seq: new(uint64), writers: map[string]*dirRecordWriter{}})

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	if out.Len() != 0 {
		t.Fatalf("stdout output = %q, want empty when routed to files", out.String())
	}
	entries, err := os.ReadDir(filepath.Join(tmpDir, "im"))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("routed files = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "im", entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(data), &record); err != nil {
		t.Fatalf("invalid routed JSON: %v", err)
	}
	if got, want := record["event_type"], "im.message.receive_v1"; got != want {
		t.Fatalf("event_type = %v, want %v", got, want)
	}
	if got, want := record["raw_payload"], `{"schema":"2.0","header":{"event_id":"ev_test","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123"}}}`; got != want {
		t.Fatalf("raw_payload = %v, want %v", got, want)
	}
	if _, err := os.Stat(fallbackDir); !os.IsNotExist(err) {
		t.Fatalf("fallback dir stat err = %v, want not exists", err)
	}
}

func TestPipeline_RawModePrettyJSONStillWritesCompactFileJSONWhenRouted(t *testing.T) {
	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	router, err := ParseRoutes([]string{"^im\\.message=dir:./im"})
	if err != nil {
		t.Fatalf("ParseRoutes() error = %v", err)
	}
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{id: genericHandlerID, eventType: "im.message.receive_v1", fn: func(_ context.Context, evt *Event) HandlerResult {
		return HandlerResult{Status: HandlerStatusHandled}
	}}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}
	p := newEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformRaw, PrettyJSON: true}, &out, &errOut, &outputRouter{router: router, fallback: ndjsonRecordWriter{w: &out}, seq: new(uint64), writers: map[string]*dirRecordWriter{}})

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	if out.Len() != 0 {
		t.Fatalf("stdout output = %q, want empty when routed to files", out.String())
	}
	entries, err := os.ReadDir(filepath.Join(tmpDir, "im"))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("routed files = %d, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "im", entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	trimmed := bytes.TrimSpace(data)
	if bytes.Contains(trimmed, []byte("\n  \"event_type\"")) {
		t.Fatalf("routed file unexpectedly used pretty JSON: %q", string(data))
	}
	var record map[string]interface{}
	if err := json.Unmarshal(trimmed, &record); err != nil {
		t.Fatalf("invalid routed JSON: %v", err)
	}
	if got, want := record["event_type"], "im.message.receive_v1"; got != want {
		t.Fatalf("event_type = %v, want %v", got, want)
	}
}

func TestPipeline_RawModePrettyJSONFallsBackToPrettyStdoutWhenUnmatched(t *testing.T) {
	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	router, err := ParseRoutes([]string{"^contact\\.=dir:./contacts"})
	if err != nil {
		t.Fatalf("ParseRoutes() error = %v", err)
	}
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{id: genericHandlerID, eventType: "im.message.receive_v1", fn: func(_ context.Context, evt *Event) HandlerResult {
		return HandlerResult{Status: HandlerStatusHandled}
	}}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}
	p := newEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformRaw, PrettyJSON: true}, &out, &errOut, &outputRouter{router: router, fallback: ndjsonRecordWriter{w: &out}, seq: new(uint64), writers: map[string]*dirRecordWriter{}})

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	got := out.String()
	if !strings.Contains(got, `"event_type":"im.message.receive_v1"`) {
		t.Fatalf("stdout output = %q, want compact fallback NDJSON", got)
	}
	if strings.Contains(got, "\n  \"event_type\"") {
		t.Fatalf("stdout output = %q, want fallback writer to ignore PrettyJSON", got)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "contacts")); !os.IsNotExist(err) {
		t.Fatalf("contacts dir stat err = %v, want not exists", err)
	}
}

func TestDeduplicateKey(t *testing.T) {
	raw := makeRawEvent("im.message.receive_v1", `{}`)
	if k := (&ImMessageProcessor{}).DeduplicateKey(raw); k != "ev_test" {
		t.Errorf("ImMessageProcessor got %q, want ev_test", k)
	}
	if k := (&GenericProcessor{}).DeduplicateKey(raw); k != "ev_test" {
		t.Errorf("GenericProcessor got %q, want ev_test", k)
	}
}

func TestPipeline_DedupePreventsDuplicateDispatch(t *testing.T) {
	var calls []string
	handler := &testEventHandler{
		id:        "event-handler",
		eventType: "im.message.receive_v1",
		result:    HandlerResult{Status: HandlerStatusHandled},
		called:    &calls,
	}
	registry := makeTestRegistry(handler, nil)
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(registry, filters, PipelineConfig{}, &out, &errOut)
	env := makeInboundEnvelope("im.message.receive_v1", `{"message":{"id":"1"}}`)

	p.Process(context.Background(), env)
	p.Process(context.Background(), env)

	if got, want := calls, []string{"event-handler"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d output lines, want 1", len(lines))
	}
}

func TestPipeline_MalformedEventUsesFallback(t *testing.T) {
	var captured *Event
	registry := NewHandlerRegistry()
	if err := registry.SetFallbackHandler(handlerFuncWith{id: "fallback", fn: func(_ context.Context, evt *Event) HandlerResult {
		captured = evt
		return HandlerResult{Status: HandlerStatusHandled}
	}}); err != nil {
		t.Fatalf("SetFallbackHandler() error = %v", err)
	}
	var out, errOut bytes.Buffer
	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{}, &out, &errOut)

	p.Process(context.Background(), InboundEnvelope{
		Source:     SourceWebhook,
		ReceivedAt: time.Unix(1700000001, 0).UTC(),
		RawPayload: []byte("not-json"),
	})

	if captured == nil {
		t.Fatal("expected fallback to capture malformed event")
	}
	if captured.EventType != "malformed" {
		t.Fatalf("fallback event type = %q, want malformed", captured.EventType)
	}
	if captured.Domain != DomainUnknown {
		t.Fatalf("fallback domain = %q, want %q", captured.Domain, DomainUnknown)
	}
	if captured.Metadata["malformed_reason"] != "invalid_json" {
		t.Fatalf("malformed_reason = %v, want invalid_json", captured.Metadata["malformed_reason"])
	}
	if payload, ok := captured.Payload.Data["raw_payload"].(string); !ok || payload != "not-json" {
		t.Fatalf("raw payload = %v, want not-json", captured.Payload.Data["raw_payload"])
	}
}

func TestPipeline_RawModeMalformedEventPreservesRawPayload(t *testing.T) {
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.SetFallbackHandler(handlerFuncWith{
		id: "fallback",
		fn: func(_ context.Context, evt *Event) HandlerResult {
			return HandlerResult{Status: HandlerStatusHandled}
		},
	}); err != nil {
		t.Fatalf("SetFallbackHandler() error = %v", err)
	}

	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformRaw}, &out, &errOut)
	p.Process(context.Background(), InboundEnvelope{
		Source:     SourceWebhook,
		ReceivedAt: time.Unix(1700000001, 0).UTC(),
		RawPayload: []byte("not-json"),
	})

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &record); err != nil {
		t.Fatalf("invalid raw-mode malformed output: %v", err)
	}
	if record["event_type"] != "malformed" {
		t.Fatalf("event_type = %v", record["event_type"])
	}
	if record["raw_payload"] != "not-json" {
		t.Fatalf("raw_payload = %v", record["raw_payload"])
	}
	if record["status"] != "handled" {
		t.Fatalf("status = %v", record["status"])
	}
}

func TestPipeline_RawModePrettyJSONWritesIndentedObject(t *testing.T) {
	var out, errOut bytes.Buffer
	registry := NewHandlerRegistry()
	if err := registry.RegisterEventHandler(handlerFuncWith{
		id:        "raw-handler",
		eventType: "im.message.receive_v1",
		fn: func(_ context.Context, evt *Event) HandlerResult {
			return HandlerResult{Status: HandlerStatusHandled}
		},
	}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}

	p := NewEventPipeline(registry, NewFilterChain(), PipelineConfig{Mode: TransformRaw, PrettyJSON: true}, &out, &errOut)
	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{"message":{"message_id":"om_123"}}`))

	got := out.String()
	if !strings.Contains(got, "\n  \"event_type\"") {
		t.Fatalf("pretty JSON output = %q, want indented object", got)
	}
}

// --- Pipeline: Quiet ---

func TestPipeline_Quiet(t *testing.T) {
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(NewHandlerRegistry(), filters,
		PipelineConfig{Quiet: true}, &out, &errOut)

	p.Process(context.Background(), makeInboundEnvelope("im.message.receive_v1", `{}`))

	if errOut.Len() != 0 {
		t.Errorf("quiet mode should suppress stderr, got: %s", errOut.String())
	}
}

// --- stderrLogger ---

func TestStderrLogger(t *testing.T) {
	var buf bytes.Buffer
	l := &stderrLogger{w: &buf, quiet: false}

	l.Debug(context.Background(), "debug msg")
	if buf.Len() != 0 {
		t.Error("Debug should always be suppressed")
	}

	l.Info(context.Background(), "info msg")
	if !strings.Contains(buf.String(), "info msg") {
		t.Error("Info should print when not quiet")
	}
	buf.Reset()

	l.Warn(context.Background(), "warn msg")
	if !strings.Contains(buf.String(), "warn msg") {
		t.Error("Warn should always print")
	}
	buf.Reset()

	l.Error(context.Background(), "error msg")
	if !strings.Contains(buf.String(), "error msg") {
		t.Error("Error should always print")
	}
}

func TestStderrLogger_Quiet(t *testing.T) {
	var buf bytes.Buffer
	l := &stderrLogger{w: &buf, quiet: true}

	l.Info(context.Background(), "info msg")
	if buf.Len() != 0 {
		t.Error("Info should be suppressed when quiet")
	}

	l.Warn(context.Background(), "warn msg")
	if !strings.Contains(buf.String(), "warn msg") {
		t.Error("Warn should print even when quiet")
	}
}

// --- RegexFilter.String ---

func TestRegexFilter_String(t *testing.T) {
	f, _ := NewRegexFilter("im\\..*")
	if f.String() != "im\\..*" {
		t.Errorf("String() = %v", f.String())
	}
}

// --- WindowStrategy ---

func TestWindowStrategy(t *testing.T) {
	im := &ImMessageProcessor{}
	if im.WindowStrategy() != (WindowConfig{}) {
		t.Error("should return zero WindowConfig")
	}
	gen := &GenericProcessor{}
	if gen.WindowStrategy() != (WindowConfig{}) {
		t.Error("should return zero WindowConfig")
	}
}

// --- Shortcuts ---

func TestShortcuts(t *testing.T) {
	s := Shortcuts()
	if len(s) == 0 {
		t.Fatal("should return at least one shortcut")
	}
	if s[0].Command != "+subscribe" {
		t.Errorf("first shortcut command = %q", s[0].Command)
	}
}

// --- Compact unmarshal error fallback ---

func TestImMessageProcessor_CompactUnmarshalError(t *testing.T) {
	p := &ImMessageProcessor{}
	raw := makeRawEvent("im.message.receive_v1", `not valid json`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent)
	if !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
	if result.Header.EventType != "im.message.receive_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

func TestImMessageProcessor_CompactInteractiveMatchesHandlerFallback(t *testing.T) {
	p := &ImMessageProcessor{}
	raw := makeRawEvent("im.message.receive_v1", `{
		"message": {
			"message_id": "om_interactive",
			"message_type": "interactive",
			"content": "{\"type\":\"template\"}"
		}
	}`)

	result := p.Transform(context.Background(), raw, TransformCompact)

	out, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("interactive compact conversion should follow handler fallback output, got %T", result)
	}
	evt, ok := legacyEventFromRaw(raw)
	if !ok {
		t.Fatal("legacyEventFromRaw() = !ok, want normalized event")
	}
	want, ok := p.Handle(context.Background(), evt).Output.(map[string]interface{})
	if !ok {
		t.Fatal("handler fallback output should be compact map")
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("transform output = %#v, want handler output %#v", out, want)
	}
	rawPayload, _ := out["raw_payload"].(string)
	if !strings.Contains(rawPayload, `"message_type":"interactive"`) {
		t.Fatalf("raw_payload = %q, want original interactive payload", rawPayload)
	}
}

func TestGenericProcessor_CompactUnmarshalError(t *testing.T) {
	p := &GenericProcessor{}
	raw := makeRawEvent("some.type", `not valid json`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent)
	if !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
	if result.Header.EventType != "some.type" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

type testHandler struct {
	id        string
	eventType string
	domain    string
}

func (h testHandler) ID() string        { return h.id }
func (h testHandler) EventType() string { return h.eventType }
func (h testHandler) Domain() string    { return h.domain }
func (h testHandler) Handle(context.Context, *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled}
}

type handlerFunc string

func (h handlerFunc) ID() string        { return string(h) }
func (h handlerFunc) EventType() string { return "" }
func (h handlerFunc) Domain() string    { return "" }
func (h handlerFunc) Handle(context.Context, *Event) HandlerResult {
	return HandlerResult{Status: HandlerStatusHandled}
}

func (h handlerFunc) with(fn func(context.Context, *Event) HandlerResult) EventHandler {
	return handlerFuncWith{id: string(h), fn: fn}
}

type handlerFuncWith struct {
	id        string
	eventType string
	domain    string
	fn        func(context.Context, *Event) HandlerResult
}

func (h handlerFuncWith) ID() string        { return h.id }
func (h handlerFuncWith) EventType() string { return h.eventType }
func (h handlerFuncWith) Domain() string    { return h.domain }
func (h handlerFuncWith) Handle(ctx context.Context, evt *Event) HandlerResult {
	return h.fn(ctx, evt)
}

func TestHandlerRegistryRejectsDuplicateFallbackHandlerID(t *testing.T) {
	r := NewHandlerRegistry()
	if err := r.RegisterEventHandler(testHandler{id: "dup", eventType: "im.message.receive_v1"}); err != nil {
		t.Fatalf("RegisterEventHandler() error = %v", err)
	}

	err := r.SetFallbackHandler(testHandler{id: "dup", eventType: "fallback", domain: "fallback"})
	if err == nil {
		t.Fatal("expected duplicate fallback handler ID to be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate handler ID: dup") {
		t.Fatalf("error = %v, want duplicate handler ID", err)
	}
}

func TestHandlerRegistrySetFallbackHandlerStoresHandler(t *testing.T) {
	r := NewHandlerRegistry()
	h := testHandler{id: "fallback", eventType: "fallback", domain: "fallback"}
	if err := r.SetFallbackHandler(h); err != nil {
		t.Fatalf("SetFallbackHandler() error = %v", err)
	}
	if got := r.FallbackHandler(); got == nil || got.ID() != h.ID() {
		t.Fatalf("FallbackHandler() = %v, want handler %q", got, h.ID())
	}
}
