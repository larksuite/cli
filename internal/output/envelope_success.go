// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import "io"

// SuccessEnvelopeOptions configures the shortcut-compatible success envelope.
type SuccessEnvelopeOptions struct {
	CommandPath string
	Identity    string
	JqExpr      string
	Out         io.Writer
	ErrOut      io.Writer
}

// SuccessEnvelopeData extracts the business payload for the standard success
// envelope from a Lark API response. Outer code/msg fields are transport
// protocol details and are intentionally not exposed as business data.
func SuccessEnvelopeData(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	data, ok := m["data"]
	if !ok || data == nil {
		return map[string]interface{}{}
	}
	return data
}

// WriteSuccessEnvelope emits the standard success envelope used by shortcuts.
// JSON output carries content-safety alerts inside the envelope. When jq is
// applied, the alert may be filtered away, so warn mode also writes stderr.
func WriteSuccessEnvelope(data interface{}, opts SuccessEnvelopeOptions) error {
	scanResult := ScanForSafety(opts.CommandPath, data, opts.ErrOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}

	env := Envelope{
		OK:       true,
		Identity: opts.Identity,
		Data:     data,
		Notice:   GetNotice(),
	}
	if scanResult.Alert != nil {
		env.ContentSafetyAlert = scanResult.Alert
	}
	if opts.JqExpr != "" {
		if scanResult.Alert != nil && opts.ErrOut != nil {
			WriteAlertWarning(opts.ErrOut, scanResult.Alert)
		}
		return JqFilter(opts.Out, env, opts.JqExpr)
	}
	PrintJson(opts.Out, env)
	return nil
}
