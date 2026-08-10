// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package processing defines how a domain processor's outcome is interpreted
// by the consume pipeline: emit exactly what the key's schema declares, or
// drop with a stated reason — never leak an undeclared shape to stdout.
package processing

import (
	"context"
	"encoding/json"
	"errors"
)

// APIClient is the narrow API surface handed to domain hooks. Identity stays
// opaque on purpose so business code cannot bypass the pre-flight checks.
type APIClient interface {
	CallAPI(ctx context.Context, method, path string, body any) (json.RawMessage, error)
}

type dropMalformedError struct{ eventType string }

func (d *dropMalformedError) Error() string {
	return "malformed payload for " + d.eventType
}

// DropMalformed signals that the payload could not be decoded into the shape
// this EventKey declares. The event is dropped instead of passing the raw
// envelope through to stdout, which would violate the declared output schema.
func DropMalformed(eventType string) error {
	return &dropMalformedError{eventType: eventType}
}

// IsDropMalformed reports whether err marks an event dropped for a malformed
// payload, letting the pipeline pick the right diagnostic without parsing
// error strings.
func IsDropMalformed(err error) bool {
	var d *dropMalformedError
	return errors.As(err, &d)
}
