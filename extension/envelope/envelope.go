// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package envelope exposes lark-cli's error-dispatch decision to embedders.
//
// Integrators that call cmd.Build and drive Execute themselves must render
// errors like the official binary so agents can parse stderr uniformly.
// DispatchError is the same function the official root dispatcher consumes,
// so error classification, exit codes, and envelope bytes match for every
// error the dispatcher receives.
//
// One narrow exception: the official root dispatcher enriches a
// need_user_authorization error with the current command's declared scopes
// (via a cmdutil.Factory it holds) before calling DispatchError. That
// enrichment depends on command context an embedder does not have, so a
// direct DispatchError call on a raw need_user_authorization error produces
// an otherwise-identical envelope without the folded-in scope hint. All other
// error categories are unaffected.
package envelope

import "github.com/larksuite/cli/internal/output"

// DispatchError classifies err exactly like lark-cli's own root dispatcher
// and returns the stderr envelope bytes (if any) together with the process
// exit code. identity is the resolved identity string ("user", "bot", or ""
// to omit the field). Typical embedder epilogue:
//
//	env, code, has := envelope.DispatchError(err, "user")
//	if has {
//		_, _ = os.Stderr.Write(env)
//	}
//	os.Exit(code)
func DispatchError(err error, identity string) (envelope []byte, exitCode int, hasEnvelope bool) {
	return output.DispatchError(err, identity)
}
