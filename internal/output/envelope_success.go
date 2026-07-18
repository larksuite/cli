// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import "io"

// SuccessEnvelopeOptions configures the shortcut-compatible success envelope.
type SuccessEnvelopeOptions struct {
	CommandPath string
	Identity    string
	DryRun      bool
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
	return NewEmitter(EmitterConfig{
		Out:            opts.Out,
		ErrOut:         opts.ErrOut,
		CommandPath:    opts.CommandPath,
		Identity:       opts.Identity,
		NoticeProvider: GetNotice,
	}).Success(data, EmitOptions{
		Format:          "",
		Raw:             false,
		JQ:              opts.JqExpr,
		DryRun:          opts.DryRun,
		JQSafetyWarning: true,
	})
}
