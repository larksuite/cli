// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"errors"
	"fmt"
	"io"
	"strings"

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

// wrapBlockError creates an ExitError for content-safety block.
func wrapBlockError(alert *extcs.Alert) error {
	rules := ""
	if alert != nil {
		rules = strings.Join(alert.MatchedRules, ", ")
	}
	return &ExitError{
		Code: ExitContentSafety,
		Detail: &ErrDetail{
			Type:    "content_safety_blocked",
			Message: fmt.Sprintf("content safety violation detected (rules: %s)", rules),
		},
	}
}

// WriteAlertWarning writes a human-readable content-safety warning to w.
// Used by non-JSON output paths (pretty, table, csv) in warn mode.
func WriteAlertWarning(w io.Writer, alert *extcs.Alert) {
	if alert == nil {
		return
	}
	fmt.Fprintf(w, "warning: content safety alert from %s (rules: %s)\n",
		alert.Provider, strings.Join(alert.MatchedRules, ", "))
}
