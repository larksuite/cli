// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
)

// NoticeProvider supplies the notice attached to a structured envelope.
// The provider is captured by an Emitter so emission never reads the global
// PendingNotice hook implicitly.
type NoticeProvider func() map[string]interface{}

// PrettyRenderer writes the human-readable representation of one result.
// colorEnabled is the terminal capability captured when the Emitter is built.
type PrettyRenderer func(w io.Writer, colorEnabled bool) error

// EmitterConfig contains command-scoped dependencies. A command constructs one
// Emitter and reuses it for its success result or streamed pages.
type EmitterConfig struct {
	Out                    io.Writer
	ErrOut                 io.Writer
	CommandPath            string
	Identity               string
	ColorEnabled           bool
	NoticeProvider         NoticeProvider
	MaxBufferedStreamBytes int
}

// EmitOptions describes one result's wire representation.
//
// The format contract is explicit: FormatJSON (the zero value) uses an
// Envelope; pretty, table, csv, and ndjson render naked business data. JQ takes
// precedence over Format and filters the JSON Envelope. Raw affects only JSON
// envelope encoding and jq's complex-value encoding. Format is a canonical
// typed value — boundaries reject unknown formats via ParseFormatStrict, so the
// Emitter never sees one and never falls back.
type EmitOptions struct {
	Raw    bool
	Meta   *Meta
	Format Format
	JQ     string
	DryRun bool
	Pretty PrettyRenderer
}

// StreamOptions describes one streamed page's wire representation. Streaming
// carries page items directly, so it deliberately exposes only the fields that
// affect a single page: the format and, for pretty, its renderer. It has no
// OK/Meta/DryRun/JQ — an ok:false envelope, metadata, dry-run, and jq all need
// the aggregated result, which the caller's pagination layer owns before it
// streams pages.
type StreamOptions struct {
	Format Format
	Pretty PrettyRenderer
}

// Emitter owns all command-scoped output dependencies and pagination state.
// It deliberately has no dependency on client or cmdutil.
type Emitter struct {
	out            io.Writer
	errOut         io.Writer
	commandPath    string
	identity       string
	colorEnabled   bool
	noticeProvider NoticeProvider
	scanCtx        scanContextFactory

	streamFormat    Format
	streamFormatSet bool
	streamPrettySet bool
	streamHasPretty bool
	streamFormatter *PaginatedFormatter
	streamMode      mode
	streamModeSet   bool
	streamBuffer    bytes.Buffer
	maxStreamBytes  int
	streamFinished  bool
	streamFinishErr error
}

const defaultMaxBufferedStreamBytes = 64 << 20

// NewEmitter constructs a command-scoped output emitter.
func NewEmitter(config EmitterConfig) *Emitter {
	errOut := config.ErrOut
	if errOut == nil {
		errOut = io.Discard
	}
	maxStreamBytes := config.MaxBufferedStreamBytes
	if maxStreamBytes <= 0 {
		maxStreamBytes = defaultMaxBufferedStreamBytes
	}
	return &Emitter{
		out:            config.Out,
		errOut:         errOut,
		commandPath:    config.CommandPath,
		identity:       config.Identity,
		colorEnabled:   config.ColorEnabled,
		noticeProvider: config.NoticeProvider,
		scanCtx:        defaultContentSafetyContext,
		maxStreamBytes: maxStreamBytes,
	}
}

// Success scans and emits one command result by composing the package's leaf
// primitives. JSON and jq use the standard envelope; pretty, table, csv, and
// ndjson render the business value directly.
func (e *Emitter) Success(data interface{}, opts EmitOptions) error {
	if !opts.Format.Valid() {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"internal: unknown output format %d", int(opts.Format))
	}
	if err := e.requireOutput(); err != nil {
		return err
	}

	if opts.JQ != "" {
		return e.emitEnvelope(data, true, opts)
	}

	switch opts.Format {
	case FormatJSON:
		return e.emitEnvelope(data, true, opts)
	case FormatPretty:
		return e.emitPretty(data, opts)
	default:
		return e.emitFormatted(data, opts.Format)
	}
}

// Value scans and emits one naked business value. It is intended for
// long-running streams and custom-format shortcuts whose public contract does
// not use the standard success envelope.
func (e *Emitter) Value(data interface{}, opts StreamOptions) error {
	if !opts.Format.Valid() {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"internal: unknown output format %d", int(opts.Format))
	}
	if err := e.requireOutput(); err != nil {
		return err
	}
	if opts.Format == FormatPretty && opts.Pretty != nil {
		return e.emitPrettyRenderer(data, opts.Pretty)
	}
	return e.emitValue(data, opts.Format)
}

// PartialFailure emits a multi-status result whose envelope honestly reports
// ok:false. It is the typed counterpart to Success for batch operations where
// some items failed but the per-item outcomes are the primary stdout output.
// JSON and jq retain the failure envelope. Other formats emit the selected
// naked representation while the caller supplies the non-zero exit signal.
func (e *Emitter) PartialFailure(data interface{}, opts EmitOptions) error {
	if !opts.Format.Valid() {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"internal: unknown output format %d", int(opts.Format))
	}
	if err := e.requireOutput(); err != nil {
		return err
	}
	if opts.JQ != "" || opts.Format == FormatJSON {
		return e.emitEnvelope(data, false, opts)
	}
	return e.Value(data, StreamOptions{Format: opts.Format, Pretty: opts.Pretty})
}

// StreamPage scans and emits one page while retaining table/csv columns from
// the first page. Streamed output carries page items directly, so it takes a
// StreamOptions (format + optional pretty renderer) rather than the full
// EmitOptions: ok/meta/dry-run/jq all need the aggregated result and are the
// caller's pagination-layer responsibility, not a per-page concern. Excluding
// jq from the type makes "jq requires aggregated output" a compile-time fact
// instead of a runtime rejection.
func (e *Emitter) StreamPage(data interface{}, opts StreamOptions) error {
	if !opts.Format.Valid() {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"internal: unknown output format %d", int(opts.Format))
	}
	if err := e.requireOutput(); err != nil {
		return err
	}
	if e.streamFinished {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"stream output is already finished")
	}
	if !e.streamFormatSet {
		e.streamFormat = opts.Format
		e.streamFormatSet = true
	} else if opts.Format != e.streamFormat {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"stream output format changed from %q to %q", e.streamFormat, opts.Format)
	}

	if opts.Format == FormatPretty {
		hasPretty := opts.Pretty != nil
		if !e.streamPrettySet {
			e.streamHasPretty = hasPretty
			e.streamPrettySet = true
		} else if hasPretty != e.streamHasPretty {
			return errs.NewInternalError(errs.SubtypeUnknown,
				"stream pretty renderer availability changed between pages")
		}
		if opts.Pretty != nil {
			var buf bytes.Buffer
			if err := opts.Pretty(&buf, e.colorEnabled); err != nil {
				return wrapOutputError("render", err)
			}
			return e.emitStreamBuffer(data, &buf)
		}
		// Commands without a curated pretty renderer use the generic table
		// representation. This keeps --format pretty truthful without requiring
		// every shortcut to duplicate a renderer.
		opts.Format = FormatTable
	}

	if e.streamFormatter == nil {
		e.streamFormatter = NewPaginatedFormatter(nil, opts.Format)
	}

	// Render this page, then scan the exact bytes before writing: a rule match
	// can form in the rendered page (joined table cells, adjacent objects) even
	// when no single value matches.
	var buf bytes.Buffer
	e.streamFormatter.W = &buf
	if err := e.streamFormatter.WritePage(data); err != nil {
		return wrapOutputError("render", err)
	}
	return e.emitStreamBuffer(data, &buf)
}

// FinishStream commits output buffered by StreamPage in block mode. Warn mode
// remains incremental: each page is scanned and written by StreamPage. Callers
// must invoke FinishStream after the final page, including when pagination ends
// with an API error and partial block-mode output should remain visible.
func (e *Emitter) FinishStream() error {
	if e.streamFinished {
		return e.streamFinishErr
	}
	e.streamFinished = true
	if !e.streamModeSet || e.streamMode != modeBlock || e.streamBuffer.Len() == 0 {
		return nil
	}
	e.streamFinishErr = e.emitScannedBufferMode(&e.streamBuffer, e.streamMode)
	return e.streamFinishErr
}

func (e *Emitter) emitEnvelope(data interface{}, ok bool, opts EmitOptions) error {
	m := modeFromEnv(e.errOut)
	env := Envelope{
		OK:       ok,
		Identity: e.identity,
		DryRun:   opts.DryRun,
		Data:     data,
		Meta:     opts.Meta,
		Notice:   e.notice(),
	}

	if opts.JQ != "" {
		sourceScan := e.scanForSafetyMode(data, false, m)
		if sourceScan.Blocked {
			return sourceScan.BlockErr
		}
		if sourceScan.Alert != nil {
			env.ContentSafetyAlert = sourceScan.Alert
		}
		// Buffer the jq output manually so jq's own typed error (a validation
		// error for a bad expression, an api error for a runtime failure) is
		// returned unchanged; only a genuine stdout write failure is wrapped as
		// an internal output error.
		var buf bytes.Buffer
		var jqErr error
		if opts.Raw {
			jqErr = JqFilterRaw(&buf, env, opts.JQ)
		} else {
			jqErr = JqFilter(&buf, env, opts.JQ)
		}
		if jqErr != nil {
			return jqErr
		}
		var renderedScan ScanResult
		if !sourceScan.scanFailed {
			renderedScan = e.scanRenderedBufferMode(&buf, m)
		}
		if renderedScan.Blocked {
			return renderedScan.BlockErr
		}
		alert := mergeSafetyAlerts(sourceScan.Alert, renderedScan.Alert)
		if alert != nil {
			if err := WriteAlertWarning(e.errOut, alert); err != nil {
				return wrapOutputError("write", err)
			}
		}
		if _, err := io.Copy(e.out, &buf); err != nil {
			return wrapOutputError("write", err)
		}
		return nil
	}

	// Scan both representations. The structured scan detects content changed by
	// JSON escaping, while the rendered scan detects matches formed across
	// serialized fields.
	sourceScan := e.scanForSafetyMode(data, false, m)
	if sourceScan.Blocked {
		return sourceScan.BlockErr
	}

	var buf bytes.Buffer
	if err := renderEnvelope(&buf, env, opts.Raw); err != nil {
		return wrapOutputError("render", err)
	}
	var renderedScan ScanResult
	if !sourceScan.scanFailed {
		renderedScan = e.scanRenderedBufferMode(&buf, m)
	}
	if renderedScan.Blocked {
		return renderedScan.BlockErr
	}
	if alert := mergeSafetyAlerts(sourceScan.Alert, renderedScan.Alert); alert != nil {
		env.ContentSafetyAlert = alert
		buf.Reset()
		if err := renderEnvelope(&buf, env, opts.Raw); err != nil {
			return wrapOutputError("render", err)
		}
	}
	if _, err := io.Copy(e.out, &buf); err != nil {
		return wrapOutputError("write", err)
	}
	return nil
}

func (e *Emitter) emitPretty(data interface{}, opts EmitOptions) error {
	if opts.Pretty != nil {
		return e.emitPrettyRenderer(data, opts.Pretty)
	}

	return e.emitFormatted(data, FormatPretty)
}

func (e *Emitter) emitPrettyRenderer(data interface{}, renderer PrettyRenderer) error {
	// Buffer pretty output so the safety scan sees the exact text that will be
	// written to stdout, including anything captured by the opaque renderer.
	var buf bytes.Buffer
	if err := renderer(&buf, e.colorEnabled); err != nil {
		return wrapOutputError("render", err)
	}
	return e.emitSourceAndRenderedBufferMode(data, &buf, modeFromEnv(e.errOut))
}

// emitFormatted renders naked business data for ndjson, table, csv, and the
// generic pretty representation. Success routes FormatJSON to the envelope and
// curated pretty output to its renderer.
func (e *Emitter) emitFormatted(data interface{}, format Format) error {
	var buf bytes.Buffer
	if err := WriteFormatted(&buf, data, format); err != nil {
		return wrapOutputError("render", err)
	}
	return e.emitSourceAndRenderedBufferMode(data, &buf, modeFromEnv(e.errOut))
}

func (e *Emitter) emitValue(data interface{}, format Format) error {
	var buf bytes.Buffer
	var err error
	switch format {
	case FormatJSON:
		err = WriteJSON(&buf, data)
	case FormatNDJSON:
		err = WriteNDJSON(&buf, data)
	case FormatTable:
		err = WriteTable(&buf, data)
	case FormatCSV:
		err = WriteCSV(&buf, data)
	case FormatPretty:
		err = WriteFormatted(&buf, data, format)
	default:
		return errs.NewInternalError(errs.SubtypeUnknown,
			"internal: unknown output format %d", int(format))
	}
	if err != nil {
		return wrapOutputError("render", err)
	}
	return e.emitSourceAndRenderedBufferMode(data, &buf, modeFromEnv(e.errOut))
}

func (e *Emitter) emitScannedBufferMode(buf *bytes.Buffer, m mode) error {
	scanResult := e.scanRenderedBufferMode(buf, m)
	return e.emitBufferAfterScan(buf, scanResult)
}

func (e *Emitter) emitSourceAndRenderedBufferMode(data interface{}, buf *bytes.Buffer, m mode) error {
	scanResult := e.scanSourceAndRenderedBufferMode(data, buf, m)
	return e.emitBufferAfterScan(buf, scanResult)
}

func (e *Emitter) emitBufferAfterScan(buf *bytes.Buffer, scanResult ScanResult) error {
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		if err := WriteAlertWarning(e.errOut, scanResult.Alert); err != nil {
			return wrapOutputError("write", err)
		}
	}
	if _, err := io.Copy(e.out, buf); err != nil {
		return wrapOutputError("write", err)
	}
	return nil
}

func (e *Emitter) emitStreamBuffer(data interface{}, buf *bytes.Buffer) error {
	if !e.streamModeSet {
		e.streamMode = modeFromEnv(e.errOut)
		e.streamModeSet = true
	}
	switch e.streamMode {
	case modeWarn:
		return e.emitSourceAndRenderedBufferMode(data, buf, e.streamMode)
	case modeBlock:
		sourceScan := e.scanForSafetyMode(data, false, e.streamMode)
		if sourceScan.Blocked {
			return sourceScan.BlockErr
		}
		if buf.Len() > e.maxStreamBytes-e.streamBuffer.Len() {
			return errs.NewContentSafetyError(errs.SubtypeContentSafety,
				"content-safety scan input exceeds the %d-byte stream limit; blocked",
				e.maxStreamBytes).
				WithHint("reduce --page-limit or request fewer records")
		}
		_, _ = e.streamBuffer.Write(buf.Bytes())
		return nil
	}
	if _, err := io.Copy(e.out, buf); err != nil {
		return wrapOutputError("write", err)
	}
	return nil
}

func (e *Emitter) scanSourceAndRenderedBufferMode(data interface{}, buf *bytes.Buffer, m mode) ScanResult {
	sourceScan := e.scanForSafetyMode(data, false, m)
	if sourceScan.Blocked || sourceScan.scanFailed {
		return sourceScan
	}
	renderedScan := e.scanRenderedBufferMode(buf, m)
	if renderedScan.Blocked {
		return renderedScan
	}
	renderedScan.Alert = mergeSafetyAlerts(sourceScan.Alert, renderedScan.Alert)
	return renderedScan
}

func (e *Emitter) scanRenderedBufferMode(buf *bytes.Buffer, m mode) ScanResult {
	return e.scanForSafetyMode(buf.String(), true, m)
}

func renderEnvelope(w io.Writer, env Envelope, raw bool) error {
	if raw {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	}
	return WriteJSON(w, env)
}

func mergeSafetyAlerts(first, second *extcs.Alert) *extcs.Alert {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	rules := make(map[string]struct{}, len(first.MatchedRules)+len(second.MatchedRules))
	for _, rule := range first.MatchedRules {
		rules[rule] = struct{}{}
	}
	for _, rule := range second.MatchedRules {
		rules[rule] = struct{}{}
	}
	mergedRules := make([]string, 0, len(rules))
	for rule := range rules {
		mergedRules = append(mergedRules, rule)
	}
	sort.Strings(mergedRules)
	provider := first.Provider
	if provider == "" {
		provider = second.Provider
	}
	return &extcs.Alert{Provider: provider, MatchedRules: mergedRules}
}

func (e *Emitter) scanForSafetyMode(data interface{}, fullText bool, m mode) ScanResult {
	return scanForSafetyMode(e.commandPath, data, e.errOut, fullText, m, e.scanCtx)
}

func wrapOutputError(op string, err error) error {
	return errs.NewInternalError(errs.SubtypeUnknown, "failed to %s command output", op).WithCause(err)
}

func (e *Emitter) notice() map[string]interface{} {
	if e.noticeProvider == nil {
		return nil
	}
	return e.noticeProvider()
}

func (e *Emitter) requireOutput() error {
	if e == nil || e.out == nil {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"success output writer is not configured")
	}
	return nil
}
