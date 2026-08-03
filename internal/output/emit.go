// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
)

// ScanResult holds the output of ScanForSafety.
type ScanResult struct {
	Alert      *extcs.Alert
	Blocked    bool
	BlockErr   error
	scanFailed bool
}

// ScanForSafety scans structured response data.
func ScanForSafety(cmdPath string, data any, errOut io.Writer) ScanResult {
	return scanForSafetyMode(cmdPath, data, errOut, false, modeFromEnv(errOut), defaultContentSafetyContext)
}

func scanForSafetyMode(cmdPath string, data any, errOut io.Writer, fullText bool, m mode, newScanContext scanContextFactory) ScanResult {
	alert, csErr := runContentSafety(cmdPath, data, errOut, fullText, m, newScanContext)
	if errors.Is(csErr, errBlocked) {
		return ScanResult{
			Alert:    alert,
			Blocked:  true,
			BlockErr: wrapBlockError(alert),
		}
	}
	if errors.Is(csErr, errScanIncomplete) {
		return ScanResult{
			Blocked:  true,
			BlockErr: wrapScanIncompleteError(csErr),
		}
	}
	if errors.Is(csErr, errScanFailed) {
		return ScanResult{scanFailed: true}
	}
	return ScanResult{Alert: alert}
}

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

func wrapScanIncompleteError(cause error) error {
	message := "content-safety scan did not complete; blocked (block mode)"
	if errors.Is(cause, context.DeadlineExceeded) {
		message = "content-safety scan did not complete in time; blocked (block mode)"
	}
	return errs.NewContentSafetyError(errs.SubtypeContentSafety, "%s", message).
		WithCause(cause)
}

// WriteAlertWarning writes a content-safety warning.
func WriteAlertWarning(w io.Writer, alert *extcs.Alert) error {
	if alert == nil {
		return nil
	}
	_, err := fmt.Fprintf(w, "warning: content safety alert from %s (rules: %s)\n",
		alert.Provider, strings.Join(alert.MatchedRules, ", "))
	return err
}
