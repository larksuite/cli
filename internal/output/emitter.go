// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/larksuite/cli/errs"
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
	Out            io.Writer
	ErrOut         io.Writer
	CommandPath    string
	Identity       string
	ColorEnabled   bool
	NoticeProvider NoticeProvider
}

// EmitOptions describes one result's wire representation.
//
// The format contract is explicit: JSON (including the empty default) uses an
// Envelope; pretty, table, csv, and ndjson render naked business data. JQ takes
// precedence over Format and filters the JSON Envelope. Raw affects only JSON
// envelope encoding and jq's complex-value encoding.
//
// JQSafetyWarning preserves the legacy difference between RuntimeContext.emit
// (false) and WriteSuccessEnvelope (true) until their callers are migrated.
type EmitOptions struct {
	Raw             bool
	OK              bool
	Meta            *Meta
	Format          string
	JQ              string
	DryRun          bool
	Pretty          PrettyRenderer
	JQSafetyWarning bool
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

	streamFormat    string
	streamFormatter *PaginatedFormatter
}

// NewEmitter constructs a command-scoped output emitter.
func NewEmitter(config EmitterConfig) *Emitter {
	return &Emitter{
		out:            config.Out,
		errOut:         config.ErrOut,
		commandPath:    config.CommandPath,
		identity:       config.Identity,
		colorEnabled:   config.ColorEnabled,
		noticeProvider: config.NoticeProvider,
	}
}

// Success scans and emits one command result by composing the package's leaf
// primitives. JSON and jq use the standard envelope; pretty, table, csv, and
// ndjson render the business value directly.
func (e *Emitter) Success(data interface{}, opts EmitOptions) error {
	if err := e.requireOutput(); err != nil {
		return err
	}

	if opts.JQ != "" {
		return e.emitEnvelope(data, opts)
	}

	switch opts.Format {
	case "", "json":
		return e.emitEnvelope(data, opts)
	case "pretty":
		return e.emitPretty(data, opts)
	default:
		return e.emitFormatted(data, opts.Format)
	}
}

// StreamPage scans and emits one page while retaining table/csv columns from
// the first page. Streamed output carries page items directly and therefore
// does not use OK, Meta, DryRun, or notices from EmitOptions.
func (e *Emitter) StreamPage(data interface{}, opts EmitOptions) error {
	if err := e.requireOutput(); err != nil {
		return err
	}
	if opts.JQ != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"jq requires aggregated pagination output").WithParam("--jq")
	}

	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil && e.errOut != nil {
		WriteAlertWarning(e.errOut, scanResult.Alert)
	}

	if opts.Format == "pretty" {
		if opts.Pretty == nil {
			return errs.NewInternalError(errs.SubtypeUnknown,
				"pretty output requires a renderer")
		}
		return opts.Pretty(e.out, e.colorEnabled)
	}

	format, known := ParseFormat(opts.Format)
	if !known && e.streamFormatter == nil && e.errOut != nil {
		fmt.Fprintf(e.errOut, "warning: unknown format %q, falling back to json\n", opts.Format)
	}
	if e.streamFormatter == nil {
		e.streamFormat = opts.Format
		e.streamFormatter = NewPaginatedFormatter(e.out, format)
	} else if opts.Format != e.streamFormat {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"stream output format changed from %q to %q", e.streamFormat, opts.Format)
	}

	e.streamFormatter.FormatPage(data)
	return nil
}

func (e *Emitter) emitEnvelope(data interface{}, opts EmitOptions) error {
	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}

	env := Envelope{
		OK:       opts.OK,
		Identity: e.identity,
		DryRun:   opts.DryRun,
		Data:     data,
		Meta:     opts.Meta,
		Notice:   e.notice(),
	}
	if scanResult.Alert != nil {
		env.ContentSafetyAlert = scanResult.Alert
	}

	if opts.JQ != "" {
		if scanResult.Alert != nil && opts.JQSafetyWarning && e.errOut != nil {
			WriteAlertWarning(e.errOut, scanResult.Alert)
		}
		if opts.Raw {
			return JqFilterRaw(e.out, env, opts.JQ)
		}
		return JqFilter(e.out, env, opts.JQ)
	}

	if opts.Raw {
		enc := json.NewEncoder(e.out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	}
	PrintJson(e.out, env)
	return nil
}

func (e *Emitter) emitPretty(data interface{}, opts EmitOptions) error {
	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil && e.errOut != nil {
		WriteAlertWarning(e.errOut, scanResult.Alert)
	}
	if opts.Pretty != nil {
		return opts.Pretty(e.out, e.colorEnabled)
	}

	// RuntimeContext.outFormat falls back through Out/OutRaw when no pretty
	// renderer is supplied. Keep that second scan visible in the leaf contract
	// until production callers are migrated and the legacy behavior is removed.
	return e.emitEnvelope(data, opts)
}

func (e *Emitter) emitFormatted(data interface{}, rawFormat string) error {
	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil && e.errOut != nil {
		WriteAlertWarning(e.errOut, scanResult.Alert)
	}

	format, known := ParseFormat(rawFormat)
	if !known && e.errOut != nil {
		fmt.Fprintf(e.errOut, "warning: unknown format %q, falling back to json\n", rawFormat)
	}
	if format == FormatJSON {
		e.printLegacyDataJSON(data)
		return nil
	}
	FormatValue(e.out, data, format)
	return nil
}

type emitterDataMap map[string]interface{}

// printLegacyDataJSON matches FormatValue's JSON branch while sourcing notice
// data from this Emitter instead of PrintJson's global PendingNotice hook.
func (e *Emitter) printLegacyDataJSON(data interface{}) {
	if m, ok := data.(map[string]interface{}); ok {
		if _, isEnvelope := m["ok"]; isEnvelope {
			if notice := e.notice(); notice != nil {
				m["_notice"] = notice
			}
		}
		// The named map retains identical JSON bytes while preventing PrintJson
		// from consulting its legacy global notice hook a second time.
		PrintJson(e.out, emitterDataMap(m))
		return
	}
	PrintJson(e.out, data)
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
