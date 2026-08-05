// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
)

// ScanResult holds the output of ScanForSafety.
type ScanResult struct {
	Alert    *extcs.Alert
	Blocked  bool
	BlockErr error
}

// ScanForSafety runs content-safety scanning on the given data.
// cmdPath is the raw cobra CommandPath().
// When MODE=off, no provider registered, or the command is not allowlisted,
// returns a zero ScanResult.
func ScanForSafety(cmdPath string, data any, errOut io.Writer) ScanResult {
	alert, csErr := runContentSafety(cmdPath, data, errOut)
	if errors.Is(csErr, errBlocked) {
		return ScanResult{
			Alert:    alert,
			Blocked:  true,
			BlockErr: wrapBlockError(alert),
		}
	}
	return ScanResult{Alert: alert}
}

// wrapBlockError creates a typed error for content-safety block.
func wrapBlockError(alert *extcs.Alert) error {
	var matchedRules []string
	if alert != nil {
		matchedRules = alert.MatchedRules
	}
	return errs.NewContentSafetyError(errs.SubtypeContentSafety,
		"content safety violation detected (rules: %s)", strings.Join(matchedRules, ", ")).
		WithRules(matchedRules...).
		WithCause(errBlocked)
}

// WriteAlertWarning writes a human-readable content-safety warning to w.
// Used by non-JSON output paths (pretty, table, csv) in warn mode.
func WriteAlertWarning(w io.Writer, alert *extcs.Alert) error {
	if alert == nil {
		return nil
	}
	_, err := fmt.Fprintf(w, "warning: content safety alert from %s (rules: %s)\n",
		alert.Provider, strings.Join(alert.MatchedRules, ", "))
	return err
}

// writePaginationDiagnostic reports a record stream's pagination outcome on the
// diagnostics stream, as one JSON object per line.
//
// A record stream has no envelope to carry meta, so without this a result
// truncated by --page-limit is byte-identical to a complete one — the reader
// cannot tell "these are all the records" from "these are the first 500". It is
// JSON rather than prose because the reader that needs it is a program.
func writePaginationDiagnostic(w io.Writer, meta PaginationMeta) error {
	payload := struct {
		Diagnostic string `json:"_diagnostic"`
		PaginationMeta
	}{Diagnostic: "pagination", PaginationMeta: meta}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return wrapOutputError("render", err)
	}
	if _, err := fmt.Fprintf(w, "%s\n", encoded); err != nil {
		return wrapOutputError("write", err)
	}
	return nil
}
