// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/larksuite/cli/internal/output"
)

const dedupTTL = 5 * time.Minute

// PipelineConfig configures the event processing pipeline.
type PipelineConfig struct {
	Mode       TransformMode // determined by --compact flag
	Quiet      bool          // --quiet: suppress stderr status messages
	PrettyJSON bool
}

// EventPipeline chains normalize -> match -> resolve -> filter -> dedupe -> dispatch.
type EventPipeline struct {
	registry     *HandlerRegistry
	filters      *FilterChain
	config       PipelineConfig
	deduper      *Deduper
	dispatcher   *Dispatcher
	dispatchedN  int64
	out          io.Writer
	errOut       io.Writer
	recordWriter OutputRecordWriter
}

// NewEventPipeline builds an event processing pipeline.
func NewEventPipeline(
	registry *HandlerRegistry,
	filters *FilterChain,
	config PipelineConfig,
	out, errOut io.Writer,
) *EventPipeline {
	return newEventPipeline(registry, filters, config, out, errOut, nil)
}

func newEventPipeline(
	registry *HandlerRegistry,
	filters *FilterChain,
	config PipelineConfig,
	out, errOut io.Writer,
	recordWriter OutputRecordWriter,
) *EventPipeline {
	if registry == nil {
		registry = NewHandlerRegistry()
	}
	return &EventPipeline{
		registry:     registry,
		filters:      filters,
		config:       config,
		deduper:      NewDeduper(dedupTTL),
		dispatcher:   NewDispatcher(registry),
		out:          out,
		errOut:       errOut,
		recordWriter: recordWriter,
	}
}

func (p *EventPipeline) infof(format string, args ...interface{}) {
	if !p.config.Quiet {
		fmt.Fprintf(p.errOut, format+"\n", args...)
	}
}

// EventCount returns the number of dispatch records written by the pipeline.
func (p *EventPipeline) EventCount() int64 {
	if p == nil {
		return 0
	}
	return p.dispatchedN
}

// Process is the pipeline entry point.
func (p *EventPipeline) Process(ctx context.Context, env InboundEnvelope) {
	evt, err := NormalizeEnvelope(env)
	if err != nil {
		evt = malformedFallbackEvent(env, err)
	}

	match, ok := MatchRawEventType(evt)
	if !ok {
		p.dispatch(ctx, evt)
		return
	}

	evt.EventType = match.EventType
	if evt.Domain == "" {
		evt.Domain = ResolveDomain(evt)
	}

	if !p.filters.Allow(evt.EventType) {
		return
	}

	if p.deduper.Seen(evt.IdempotencyKey, env.ReceivedAt) {
		p.infof("%s[dedup]%s %s (key=%s)", output.Dim, output.Reset, evt.EventType, evt.IdempotencyKey)
		return
	}

	p.dispatch(ctx, evt)
}

func (p *EventPipeline) dispatch(ctx context.Context, evt *Event) {
	result := p.dispatcher.Dispatch(ctx, evt)
	for _, record := range result.Results {
		p.dispatchedN++
		var entry map[string]interface{}
		if p.config.Mode == TransformRaw && p.recordWriter != nil {
			entry = rawModeRecord(evt, record)
			if len(evt.Metadata) > 0 {
				entry["metadata"] = evt.Metadata
			}
			if reason := summarizeDispatchReason(result); reason != "" {
				entry["reason"] = reason
			}
			if err := p.recordWriter.WriteRecord(evt.EventType, entry); err != nil {
				output.PrintError(p.errOut, fmt.Sprintf("write failed: %v", err))
				return
			}
			continue
		}
		if p.config.Mode == TransformRaw {
			entry = rawModeRecord(evt, record)
		} else {
			entry = compactModeRecord(evt, record)
		}
		if err := p.writeRecord(entry); err != nil {
			output.PrintError(p.errOut, fmt.Sprintf("write failed: %v", err))
			return
		}
	}
}

func compactModeRecord(evt *Event, record DispatchRecord) map[string]interface{} {
	entry := map[string]interface{}{
		"event_type": evt.EventType,
		"handler_id": record.HandlerID,
		"status":     record.Status,
	}
	mergeHandlerOutput(entry, record.Output)
	if evt.Domain != "" {
		entry["domain"] = evt.Domain
	}
	if evt.EventID != "" {
		entry["event_id"] = evt.EventID
	}
	if evt.IdempotencyKey != "" {
		entry["idempotency_key"] = evt.IdempotencyKey
	}
	if record.Reason != "" {
		entry["reason"] = record.Reason
	}
	if record.Err != nil {
		entry["error"] = record.Err.Error()
	}
	return entry
}

func rawModeRecord(evt *Event, record DispatchRecord) map[string]interface{} {
	entry := map[string]interface{}{
		"event_type":      evt.EventType,
		"status":          record.Status,
		"payload":         evt.Payload.Data,
		"raw_payload":     string(evt.RawPayload),
		"source":          evt.Source,
		"idempotency_key": evt.IdempotencyKey,
	}
	if evt.Domain != "" {
		entry["domain"] = evt.Domain
	}
	if evt.EventID != "" {
		entry["event_id"] = evt.EventID
	}
	if record.Reason != "" {
		entry["reason"] = record.Reason
	}
	if record.Err != nil {
		entry["error"] = record.Err.Error()
	}
	return entry
}

func malformedFallbackEvent(env InboundEnvelope, err error) *Event {
	reason := "malformed"
	if malformed, ok := err.(*MalformedEventError); ok && malformed.Reason != "" {
		reason = malformed.Reason
	}

	metadata := map[string]interface{}{
		"received_at":      env.ReceivedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		"malformed_reason": reason,
	}
	if err != nil {
		metadata["normalization_error"] = err.Error()
	}

	payload := NormalizedPayload{Data: map[string]interface{}{"raw_payload": string(env.RawPayload)}}
	return &Event{
		Source:         env.Source,
		EventType:      "malformed",
		Domain:         DomainUnknown,
		Payload:        payload,
		RawPayload:     append([]byte(nil), env.RawPayload...),
		Metadata:       metadata,
		IdempotencyKey: buildIdempotencyKey(env.Source, "", env.RawPayload),
	}
}

func mergeHandlerOutput(entry map[string]interface{}, outputValue interface{}) {
	if entry == nil || outputValue == nil {
		return
	}
	if compact, ok := outputValue.(map[string]interface{}); ok {
		for k, v := range compact {
			entry[k] = v
		}
		return
	}
	entry["output"] = outputValue
}

func summarizeDispatchReason(result DispatchResult) string {
	if len(result.Results) == 0 {
		return ""
	}
	reason := result.Results[0].Reason
	if reason == "" {
		return ""
	}
	for _, record := range result.Results[1:] {
		if record.Reason != reason {
			return ""
		}
	}
	return reason
}

type ndjsonRecordWriter struct {
	w io.Writer
}

func (w ndjsonRecordWriter) WriteRecord(_ string, value map[string]interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = w.w.Write(append(data, '\n'))
	return err
}

func (p *EventPipeline) writeRecord(value interface{}) error {
	var (
		data []byte
		err  error
	)
	if p.config.PrettyJSON {
		data, err = json.MarshalIndent(value, "", "  ")
		if err == nil {
			data = append(data, '\n')
		}
	} else {
		data, err = json.Marshal(value)
		if err == nil {
			data = append(data, '\n')
		}
	}
	if err != nil {
		return err
	}
	_, err = p.out.Write(data)
	return err
}
