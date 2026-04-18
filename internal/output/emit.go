// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	extcs "github.com/larksuite/cli/extension/contentsafety"
)

// ScanResult holds the output of ScanForSafety.
type ScanResult struct {
	// Alert is non-nil when the provider detected an issue. Callers should
	// attach it to the output envelope (e.g. Envelope.ContentSafetyAlert).
	Alert *extcs.Alert

	// Blocked is true when MODE=block and the provider detected an issue.
	// Callers must not write any data to stdout and should return BlockErr.
	Blocked bool

	// BlockErr is the *ExitError to return when Blocked is true.
	// nil when Blocked is false.
	BlockErr error
}

// ScanForSafety runs content-safety scanning on the given data.
// This is the public entry point for call sites (e.g. runner.Out,
// runner.OutFormat) that handle their own envelope construction and
// format dispatch.
//
// cmdPath is the raw cobra CommandPath() (e.g. "lark-cli im +messages-search").
// Normalization and ALLOWLIST matching are done internally.
//
// When MODE=off, no provider is registered, or the command is not in
// ALLOWLIST, returns a zero ScanResult (Alert=nil, Blocked=false).
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

// ShortcutEmitRequest carries everything EmitShortcut needs to produce
// the final output for a shortcut command.
type ShortcutEmitRequest struct {
	CommandPath string
	Data        any
	Identity    string
	Meta        *Meta
	Format      string
	JqExpr      string
	PrettyFn    func(io.Writer)
	Out         io.Writer
	ErrOut      io.Writer
}

// EmitShortcut is the sole output path for shortcut commands.
func EmitShortcut(req ShortcutEmitRequest) error {
	alert, csErr := runContentSafety(req.CommandPath, req.Data, req.ErrOut)
	if errors.Is(csErr, errBlocked) {
		return wrapBlockError(alert)
	}

	env := Envelope{
		OK:       true,
		Identity: req.Identity,
		Data:     req.Data,
		Meta:     req.Meta,
		Notice:   GetNotice(),
	}
	if alert != nil {
		env.ContentSafetyAlert = alert
	}

	if req.JqExpr != "" {
		return JqFilter(req.Out, env, req.JqExpr)
	}

	switch strings.ToLower(strings.TrimSpace(req.Format)) {
	case "", "json":
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Fprintln(req.Out, string(b))
		return nil
	case "pretty":
		if alert != nil {
			WriteAlertWarning(req.ErrOut, alert)
		}
		if req.PrettyFn != nil {
			req.PrettyFn(req.Out)
			return nil
		}
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Fprintln(req.Out, string(b))
		return nil
	}

	format, ok := ParseFormat(req.Format)
	if !ok {
		fmt.Fprintf(req.ErrOut, "warning: unknown format %q, falling back to json\n", req.Format)
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Fprintln(req.Out, string(b))
		return nil
	}
	if alert != nil {
		WriteAlertWarning(req.ErrOut, alert)
	}
	FormatValue(req.Out, req.Data, format)
	return nil
}

// LarkResponseEmitRequest carries everything EmitLarkResponse needs to
// pass a raw Lark API response through to the user.
type LarkResponseEmitRequest struct {
	CommandPath string
	Data        any
	Format      string
	JqExpr      string
	Out         io.Writer
	ErrOut      io.Writer
}

// EmitLarkResponse is the sole output path for cmd/api and cmd/service commands.
func EmitLarkResponse(req LarkResponseEmitRequest) error {
	alert, csErr := runContentSafety(req.CommandPath, req.Data, req.ErrOut)
	if errors.Is(csErr, errBlocked) {
		return wrapBlockError(alert)
	}

	if alert != nil {
		routeLarkAlert(req, alert)
	}

	if req.JqExpr != "" {
		return JqFilter(req.Out, req.Data, req.JqExpr)
	}
	format, _ := ParseFormat(req.Format)
	FormatValue(req.Out, req.Data, format)
	return nil
}

func routeLarkAlert(req LarkResponseEmitRequest, alert *extcs.Alert) {
	canInject := req.JqExpr == "" &&
		isJSONFormat(req.Format) &&
		isLarkShapedMap(req.Data)

	if canInject {
		req.Data.(map[string]any)["_content_safety_alert"] = alert
		return
	}
	WriteAlertWarning(req.ErrOut, alert)
}

func isJSONFormat(s string) bool {
	norm := strings.ToLower(strings.TrimSpace(s))
	return norm == "" || norm == "json"
}

func isLarkShapedMap(data any) bool {
	m, ok := data.(map[string]any)
	if !ok {
		return false
	}
	_, hasCode := m["code"]
	return hasCode
}

// WriteAlertWarning writes a plain-text content-safety alert to w.
// Used by non-JSON format paths where there is no envelope field to
// carry the alert.
func WriteAlertWarning(w io.Writer, alert *extcs.Alert) {
	rules := make([]string, len(alert.Matches))
	for i, m := range alert.Matches {
		rules[i] = m.Rule
	}
	fmt.Fprintf(w,
		"warning: content safety alert from provider %q: %d rule(s) matched [%s]\n",
		alert.Provider, len(alert.Matches), strings.Join(rules, ", "))
}
