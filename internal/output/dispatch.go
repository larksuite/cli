// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"errors"
	"strings"

	"github.com/larksuite/cli/errs"
)

// DispatchError classifies err exactly like the root command dispatcher and
// returns the rendered stderr envelope (if any) together with the process
// exit code. It is the single classification path shared by lark-cli's own
// root dispatcher and the public extension/envelope facade, so embedders that
// drive Execute themselves render errors byte-identically to the official
// binary.
//
// Classification, in order:
//
//  1. nil → (nil, 0, false).
//  2. Typed errs.* carrying a Problem → (envelope, ExitCodeOf(err), true).
//     If envelope encoding fails the error falls through to branch 4 so
//     stderr is never blank.
//  3. *PartialFailureError / *BareError → (nil, signal code, false): the
//     result envelope is already on stdout; write nothing to stderr.
//  4. Remaining untyped errors: cobra usage text → invalid_argument envelope
//     with exit 2; anything else leaked past the typed boundary → internal
//     envelope with exit 5.
func DispatchError(err error, identity string) (envelope []byte, exitCode int, hasEnvelope bool) {
	if err == nil {
		return nil, 0, false
	}
	typedExit := ExitCodeOf(err)
	if env, ok := renderTypedEnvelope(err, identity); ok {
		return env, typedExit, true
	}

	var pfErr *PartialFailureError
	if errors.As(err, &pfErr) {
		return nil, pfErr.Code, false
	}
	var bareErr *BareError
	if errors.As(err, &bareErr) {
		return nil, bareErr.Code, false
	}

	var fallback error
	if isCobraUsageError(err) {
		fallback = errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err.Error()).WithCause(err)
	} else {
		fallback = errs.NewInternalError(errs.SubtypeUnknown, "%s", err.Error()).WithCause(err)
	}
	env, ok := renderTypedEnvelope(fallback, identity)
	if !ok {
		return nil, ExitCodeOf(fallback), false
	}
	return env, ExitCodeOf(fallback), true
}

// cobraUsageErrorMarkers are the stable error-text fragments cobra / pflag
// (pinned at v1.10.2) emit for usage mistakes — missing required flag, unknown
// command / flag, wrong argument count. Cobra surfaces these as plain errors,
// not a typed value we can match on, so the dispatcher recognizes them by text;
// this is the same external contract unknownFlagName already depends on. A
// residual error matching none of these has leaked the typed boundary and is
// treated as an internal fault, not a user error.
var cobraUsageErrorMarkers = []string{
	"unknown command ",
	"unknown flag: ",
	"unknown shorthand",
	"required flag(s) ",
	"flag needs an argument",
	"bad flag syntax:",
	"no such flag ",
	"invalid argument ",
	"arg(s), ", // accepts / requires N arg(s), received / only received M
}

// isCobraUsageError reports whether err is a cobra / pflag usage mistake,
// identified by the stable error text of the pinned cobra version.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	for _, m := range cobraUsageErrorMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}
