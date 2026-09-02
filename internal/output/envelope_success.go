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
// envelope from a Lark API response.
//
// Most Lark APIs wrap the business payload in "data". Some legacy/special
// APIs (e.g. /open-apis/bot/v3/info) return the payload at the top level
// under keys like "bot". When "data" is absent, preserve the full response
// body so those APIs are not rendered as an empty envelope. Bare code/msg
// responses (no business payload) still return an empty object.
func SuccessEnvelopeData(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	if data, ok := m["data"]; ok {
		if data == nil {
			return map[string]interface{}{}
		}
		return data
	}
	// Bare protocol envelope with no business payload.
	if len(m) == 2 && m["code"] != nil && m["msg"] != nil {
		return map[string]interface{}{}
	}
	// Top-level payload (e.g. "bot" from /bot/v3/info).
	return m
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
