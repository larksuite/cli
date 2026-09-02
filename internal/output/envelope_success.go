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
//
// Most endpoints wrap the payload in "data". The legacy /open-apis/bot/v3/info
// endpoint uses "bot" as that payload container; normalize its sole business
// field to the same data shape. Other non-data responses fall back to everything
// except the transport fields. A non-object body is passed through untouched so
// this function never collapses a payload to {}.
func SuccessEnvelopeData(result interface{}) interface{} {
	if result == nil {
		return map[string]interface{}{}
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	if data, ok := m["data"]; ok && data != nil {
		return data
	}
	payload := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == "code" || k == "msg" || k == "data" {
			continue
		}
		payload[k] = v
	}
	if len(payload) == 1 {
		if bot, ok := payload["bot"].(map[string]interface{}); ok {
			return bot
		}
	}
	return payload
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
