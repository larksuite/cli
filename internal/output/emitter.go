// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
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
// Envelope. Pretty and table render business data plus a human pagination
// summary when supplied; csv and ndjson keep stdout as naked records and put
// pagination metadata on the diagnostics stream. JQ takes precedence over
// Format and filters the JSON Envelope. Raw affects only JSON envelope encoding
// and jq's complex-value encoding.
//
// JQSafetyWarning preserves the legacy difference between RuntimeContext.emit
// (false) and WriteSuccessEnvelope (true) until their callers are migrated.
type EmitOptions struct {
	Raw             bool
	Meta            *Meta
	Error           interface{}
	Hint            string
	Format          string
	JQ              string
	DryRun          bool
	Pretty          PrettyRenderer
	HintToStderr    bool
	JQSafetyWarning bool
}

// StreamOptions describes one streamed page's wire representation. Streaming
// carries page items directly, so it deliberately exposes only the fields that
// affect a single page: the format and, for pretty, its renderer. It has no
// OK/Meta/DryRun/JQ — an ok:false envelope, metadata, dry-run, and jq all need
// the aggregated result, which the caller's pagination layer owns before it
// streams pages.
type StreamOptions struct {
	Format string
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

	streamFormat    string
	streamFormatter *PaginatedFormatter
}

// NewEmitter constructs a command-scoped output emitter.
func NewEmitter(config EmitterConfig) *Emitter {
	errOut := config.ErrOut
	if errOut == nil {
		errOut = io.Discard
	}
	return &Emitter{
		out:            config.Out,
		errOut:         errOut,
		commandPath:    config.CommandPath,
		identity:       config.Identity,
		colorEnabled:   config.ColorEnabled,
		noticeProvider: config.NoticeProvider,
	}
}

// Success scans and emits one command result by composing the package's leaf
// primitives. JSON and jq use the standard envelope; record formats keep their
// stdout payload free of envelope metadata.
func (e *Emitter) Success(data interface{}, opts EmitOptions) error {
	if err := e.requireOutput(); err != nil {
		return err
	}

	var err error
	if opts.JQ != "" {
		err = e.emitEnvelope(data, true, opts)
	} else if opts.Format == "pretty" {
		err = e.emitPretty(data, opts)
	} else {
		format, known := ParseFormat(opts.Format)
		if !known {
			fmt.Fprintf(e.errOut, "warning: unknown format %q, falling back to json\n", opts.Format)
			err = e.emitEnvelope(data, true, opts)
		} else {
			switch format {
			case FormatJSON:
				err = e.emitEnvelope(data, true, opts)
			case FormatTable, FormatCSV, FormatNDJSON:
				err = e.emitFormatted(data, format, opts.Meta)
			default:
				err = errs.NewInternalError(errs.SubtypeUnknown,
					"unsupported output format %q", format)
			}
		}
	}
	if err != nil {
		return err
	}
	return e.emitHint(opts)
}

// PartialFailure emits a multi-status result whose envelope honestly reports
// ok:false. It is the typed counterpart to Success for batch operations where
// some items failed but the per-item outcomes are the primary stdout output.
// Like the legacy OutPartialFailure it produces only the JSON/jq envelope; the
// caller owns the non-zero exit signal, keeping the Emitter free of exit
// semantics.
func (e *Emitter) PartialFailure(data interface{}, opts EmitOptions) error {
	if err := e.requireOutput(); err != nil {
		return err
	}
	if err := e.emitEnvelope(data, false, opts); err != nil {
		return err
	}
	return e.emitHint(opts)
}

// StreamPage scans and emits one page while retaining table/csv columns from
// the first page. Streamed output carries page items directly, so it takes a
// StreamOptions (format + optional pretty renderer) rather than the full
// EmitOptions: ok/meta/dry-run/jq all need the aggregated result and are the
// caller's pagination-layer responsibility, not a per-page concern. Excluding
// jq from the type makes "jq requires aggregated output" a compile-time fact
// instead of a runtime rejection.
func (e *Emitter) StreamPage(data interface{}, opts StreamOptions) error {
	if err := e.requireOutput(); err != nil {
		return err
	}

	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		if err := WriteAlertWarning(e.errOut, scanResult.Alert); err != nil {
			return wrapOutputError("write", err)
		}
	}

	if opts.Format == "pretty" {
		if opts.Pretty == nil {
			return errs.NewInternalError(errs.SubtypeUnknown,
				"pretty output requires a renderer")
		}
		return e.emit(func(w io.Writer) error {
			return opts.Pretty(w, e.colorEnabled)
		})
	}

	format, known := ParseFormat(opts.Format)
	if !known && e.streamFormatter == nil && e.errOut != nil {
		fmt.Fprintf(e.errOut, "warning: unknown format %q, falling back to json\n", opts.Format)
	}
	if e.streamFormatter == nil {
		e.streamFormat = opts.Format
		e.streamFormatter = NewPaginatedFormatter(nil, format)
	} else if opts.Format != e.streamFormat {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"stream output format changed from %q to %q", e.streamFormat, opts.Format)
	}

	return e.emit(func(w io.Writer) error {
		e.streamFormatter.W = w
		return e.streamFormatter.WritePage(data)
	})
}

// Hint writes recovery guidance to stderr through the same command-scoped
// output owner used for result emission.
func (e *Emitter) Hint(hint string) error {
	return e.emitHint(EmitOptions{Hint: hint, HintToStderr: true})
}

// RedactedFallback atomically emits an already allowlisted fallback envelope.
// It deliberately skips safety scanning and jq: callers use it only after
// presentation failed, and must construct the envelope from fixed public
// fields rather than from the blocked payload.
func (e *Emitter) RedactedFallback(env Envelope) error {
	if err := e.requireOutput(); err != nil {
		return err
	}
	return e.emit(func(w io.Writer) error {
		return WriteJSON(w, env)
	})
}

func (e *Emitter) emitEnvelope(data interface{}, ok bool, opts EmitOptions) error {
	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}

	env := Envelope{
		OK:       ok,
		Identity: e.identity,
		DryRun:   opts.DryRun,
		Data:     data,
		Meta:     opts.Meta,
		Error:    opts.Error,
		Hint:     opts.Hint,
		Notice:   e.notice(),
	}
	if scanResult.Alert != nil {
		env.ContentSafetyAlert = scanResult.Alert
	}

	if opts.JQ != "" {
		if scanResult.Alert != nil && opts.JQSafetyWarning {
			if err := WriteAlertWarning(e.errOut, scanResult.Alert); err != nil {
				return wrapOutputError("write", err)
			}
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
		if _, err := io.Copy(e.out, &buf); err != nil {
			return wrapOutputError("write", err)
		}
		return nil
	}

	return e.emit(func(w io.Writer) error {
		if opts.Raw {
			enc := json.NewEncoder(w)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			return enc.Encode(env)
		}
		return WriteJSON(w, env)
	})
}

func (e *Emitter) emitPretty(data interface{}, opts EmitOptions) error {
	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		if err := WriteAlertWarning(e.errOut, scanResult.Alert); err != nil {
			return wrapOutputError("write", err)
		}
	}
	if opts.Pretty != nil {
		return e.emit(func(w io.Writer) error {
			if err := opts.Pretty(w, e.colorEnabled); err != nil {
				return err
			}
			return writePaginationSummary(w, opts.Meta)
		})
	}

	// RuntimeContext.outFormat falls back through Out/OutRaw when no pretty
	// renderer is supplied. Keep that second scan visible in the leaf contract
	// until production callers are migrated and the legacy behavior is removed.
	return e.emitEnvelope(data, true, opts)
}

// emitFormatted handles only non-envelope formats. JSON, jq, and unknown-format
// fallback are resolved by Success before reaching this function, so there is
// exactly one JSON success contract: the standard Envelope.
func (e *Emitter) emitFormatted(data interface{}, format Format, meta *Meta) error {
	scanResult := ScanForSafety(e.commandPath, data, e.errOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		if err := WriteAlertWarning(e.errOut, scanResult.Alert); err != nil {
			return wrapOutputError("write", err)
		}
	}

	switch format {
	case FormatTable:
		return e.emit(func(w io.Writer) error {
			if err := WriteFormatted(w, data, format); err != nil {
				return err
			}
			return writePaginationSummary(w, meta)
		})
	case FormatCSV, FormatNDJSON:
		if err := e.emit(func(w io.Writer) error {
			return WriteFormatted(w, data, format)
		}); err != nil {
			return err
		}
		if meta == nil || meta.Pagination == nil {
			return nil
		}
		return writePaginationDiagnostic(e.errOut, *meta.Pagination)
	default:
		return errs.NewInternalError(errs.SubtypeUnknown,
			"non-envelope emitter received unsupported format %q", format)
	}
}

func writePaginationSummary(w io.Writer, meta *Meta) error {
	if meta == nil || meta.Pagination == nil {
		return nil
	}
	pagination := meta.Pagination
	status := "complete"
	if !pagination.Complete {
		status = "incomplete"
	}
	if _, err := fmt.Fprintf(w, "\nPagination: %s (%d page(s), %d item(s))", status, pagination.Pages, pagination.Items); err != nil {
		return err
	}
	if !pagination.Complete && pagination.NextToken != "" {
		if _, err := fmt.Fprintf(w, "; resume token: %q", pagination.NextToken); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w)
	return err
}

func (e *Emitter) emit(render func(io.Writer) error) error {
	var buf bytes.Buffer
	if err := render(&buf); err != nil {
		return wrapOutputError("render", err)
	}
	if _, err := io.Copy(e.out, &buf); err != nil {
		return wrapOutputError("write", err)
	}
	return nil
}

func wrapOutputError(op string, err error) error {
	return errs.NewInternalError(errs.SubtypeUnknown, "failed to %s command output", op).WithCause(err)
}

func (e *Emitter) emitHint(opts EmitOptions) error {
	if !opts.HintToStderr || opts.Hint == "" {
		return nil
	}
	if _, err := fmt.Fprintf(e.errOut, "hint: %s\n", opts.Hint); err != nil {
		return wrapOutputError("write", err)
	}
	return nil
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
