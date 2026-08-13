// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package requestcontext carries request metadata shared by runtime
// transports. It deliberately does not know which credential source or data
// plane consumes that metadata.
package requestcontext

import (
	"context"

	"github.com/larksuite/cli/internal/core"
)

type identityKey struct{}

// WithIdentity records the identity selected for a single outbound request.
func WithIdentity(ctx context.Context, identity core.Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// Identity returns the identity selected for the current outbound request.
func Identity(ctx context.Context) core.Identity {
	identity, _ := ctx.Value(identityKey{}).(core.Identity)
	return identity
}
