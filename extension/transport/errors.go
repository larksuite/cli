// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"errors"
	"fmt"
)

// ErrAborted is a sentinel matched by errors.Is on any extension-triggered
// round-trip abort. Callers that only need to know whether an error was
// caused by an extension interception should use:
//
//	if errors.Is(err, transport.ErrAborted) { ... }
var ErrAborted = errors.New("round trip aborted by extension")

// AbortError is returned by the built-in middleware when an AbortableInterceptor
// short-circuits a request via PreRoundTripE. It wraps the extension's original
// reason and carries the extension's Provider.Name() for traceability.
//
// Use errors.As to recover the typed error:
//
//	var aErr *transport.AbortError
//	if errors.As(err, &aErr) {
//	    log.Printf("blocked by %s: %v", aErr.Extension, aErr.Reason)
//	}
//
// errors.Is(err, transport.ErrAborted) also works, and errors.Is against the
// inner reason still works via Unwrap.
type AbortError struct {
	// Extension is the name of the Provider whose interceptor aborted the
	// request (from Provider.Name()). May be empty if the provider did not
	// supply a name.
	Extension string
	// Reason is the original non-nil error returned by PreRoundTripE.
	Reason error
}

func (e *AbortError) Error() string {
	if e.Extension != "" {
		return fmt.Sprintf("extension %q aborted round trip: %v", e.Extension, e.Reason)
	}
	return fmt.Sprintf("extension aborted round trip: %v", e.Reason)
}

// Unwrap lets errors.Is / errors.As traverse to the underlying Reason.
func (e *AbortError) Unwrap() error { return e.Reason }

// Is enables errors.Is(err, ErrAborted) at any nesting depth.
func (e *AbortError) Is(target error) bool { return target == ErrAborted }
