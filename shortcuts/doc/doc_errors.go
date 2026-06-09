// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// wrapDocNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapDocNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

// wrapDocInputFileErr wraps a --file Stat/read failure via the shared typed
// helper (which sets the cause) and tags it with the --file param so agents
// learn which flag to fix. The common helper is flag-agnostic, so the param is
// attached here at the Doc call site rather than mutating shared behavior.
func wrapDocInputFileErr(err error, readMsg string) error {
	wrapped := common.WrapInputStatErrorTyped(err, readMsg)
	var ve *errs.ValidationError
	if errors.As(wrapped, &ve) {
		ve.Param = "--file"
	}
	return wrapped
}
