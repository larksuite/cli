// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
)

// Format represents an output format type.
type Format int

const (
	FormatJSON Format = iota
	FormatNDJSON
	FormatTable
	FormatCSV
	FormatPretty
)

// Valid reports whether f is one of the defined output formats.
func (f Format) Valid() bool {
	return f >= FormatJSON && f <= FormatPretty
}

// ParseFormat parses a format string into a Format value.
// The second return value is false if the format string was not recognized,
// in which case FormatJSON is returned as default.
//
// Prefer ParseFormatStrict at flag boundaries so an unknown --format fails
// loudly instead of degrading to JSON. ParseFormat's lenient fallback is kept
// for internal callers that only need a best-effort classification (e.g.
// ValidateJqFlags, which folds any non-JSON — known or not — into one branch).
func ParseFormat(s string) (Format, bool) {
	switch strings.ToLower(s) {
	case "json", "":
		return FormatJSON, true
	case "ndjson":
		return FormatNDJSON, true
	case "table":
		return FormatTable, true
	case "csv":
		return FormatCSV, true
	case "pretty":
		return FormatPretty, true
	default:
		return FormatJSON, false
	}
}

// ParseFormatStrict parses a --format value into a typed Format, returning a
// typed ValidationError for any unrecognized value instead of silently falling
// back to JSON. Flag boundaries use this so an unknown format is a typed
// failure the caller cannot accidentally serve as JSON, and so the Emitter
// downstream only ever receives a canonical Format.
func ParseFormatStrict(s string) (Format, error) {
	if f, ok := ParseFormat(s); ok {
		return f, nil
	}
	return FormatJSON, errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unknown output format %q (want json, ndjson, table, csv, or pretty)", s).
		WithParam("--format")
}

// String returns the string representation of a Format.
func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatNDJSON:
		return "ndjson"
	case FormatTable:
		return "table"
	case FormatCSV:
		return "csv"
	case FormatPretty:
		return "pretty"
	default:
		return fmt.Sprintf("unknown(%d)", int(f))
	}
}
