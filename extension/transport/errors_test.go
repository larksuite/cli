// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"errors"
	"fmt"
	"testing"
)

func TestAbortError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AbortError
		want string
	}{
		{
			name: "with extension name",
			err:  &AbortError{Extension: "audit", Reason: errors.New("bad")},
			want: `extension "audit" aborted round trip: bad`,
		},
		{
			name: "without extension name",
			err:  &AbortError{Reason: errors.New("bad")},
			want: "extension aborted round trip: bad",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAbortError_Unwrap(t *testing.T) {
	reason := errors.New("bad")
	e := &AbortError{Reason: reason}
	if got := e.Unwrap(); got != reason {
		t.Fatalf("Unwrap() = %v, want %v", got, reason)
	}
}

func TestAbortError_IsErrAborted(t *testing.T) {
	e := &AbortError{Reason: errors.New("bad")}
	if !errors.Is(e, ErrAborted) {
		t.Fatal("errors.Is(e, ErrAborted) = false, want true")
	}
	// Sanity: not matched by unrelated sentinels.
	if errors.Is(e, errors.New("other")) {
		t.Fatal("errors.Is matched unrelated sentinel")
	}
}

func TestAbortError_UnwrapReachesInnerSentinel(t *testing.T) {
	// Extensions often return typed/sentinel errors; callers should still be
	// able to errors.Is against those after the middleware wraps them.
	innerSentinel := errors.New("policy-deny-42")
	e := &AbortError{Reason: fmt.Errorf("wrapped: %w", innerSentinel)}
	if !errors.Is(e, innerSentinel) {
		t.Fatal("errors.Is(e, innerSentinel) = false, want true (Unwrap chain broken)")
	}
}

func TestAbortError_As(t *testing.T) {
	reason := errors.New("bad")
	base := &AbortError{Extension: "audit", Reason: reason}

	// Direct As.
	var aErr *AbortError
	if !errors.As(base, &aErr) {
		t.Fatal("errors.As(base, *AbortError) = false")
	}
	if aErr.Extension != "audit" || aErr.Reason != reason {
		t.Fatalf("aErr = %+v, want {audit, bad}", aErr)
	}

	// Nested As: even when the *AbortError is wrapped in another error,
	// errors.As must still find it via Unwrap chain.
	wrapped := fmt.Errorf("outer: %w", base)
	var aErr2 *AbortError
	if !errors.As(wrapped, &aErr2) {
		t.Fatal("errors.As(wrapped, *AbortError) = false")
	}
	if aErr2 != base {
		t.Fatalf("aErr2 = %p, want %p", aErr2, base)
	}

	// errors.Is still matches the sentinel through the outer wrapper.
	if !errors.Is(wrapped, ErrAborted) {
		t.Fatal("errors.Is(wrapped, ErrAborted) = false via nested wrap")
	}
}

func TestErrAborted_IsItselfSentinel(t *testing.T) {
	// Guard against accidental re-assignment of ErrAborted: a bare ErrAborted
	// value should still satisfy errors.Is(err, ErrAborted) for symmetry.
	if !errors.Is(ErrAborted, ErrAborted) {
		t.Fatal("errors.Is(ErrAborted, ErrAborted) = false")
	}
}
