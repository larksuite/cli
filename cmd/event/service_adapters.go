// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"

	eventlib "github.com/larksuite/cli/internal/event"
	appconsume "github.com/larksuite/cli/internal/event/application/consume"
	"github.com/larksuite/cli/internal/event/catalog"
)

// consumeStrategies is the executable strategy set for this binary. The same
// registry is handed to catalog compilation, so a reference the compiler
// accepted is guaranteed to resolve here.
var consumeStrategies = appconsume.DefaultRegistry()

type identityResolverFunc func(ctx context.Context, entry *catalog.Entry) (string, error)

func (f identityResolverFunc) Resolve(ctx context.Context, entry *catalog.Entry) (string, error) {
	return f(ctx, entry)
}

type preflightReaderFunc func(ctx context.Context, entry *catalog.Entry, identity string) ([]appconsume.Precondition, error)

func (f preflightReaderFunc) Read(ctx context.Context, entry *catalog.Entry, identity string) ([]appconsume.Precondition, error) {
	return f(ctx, entry, identity)
}

type streamRunnerFunc func(ctx context.Context, prepare appconsume.PrepareFunc) error

func (f streamRunnerFunc) Run(ctx context.Context, prepare appconsume.PrepareFunc) error {
	return f(ctx, prepare)
}

// readPreconditions classifies the existing read-only preflight checks into
// named preconditions. Weak dependencies that could not answer stay visible
// as "unknown" instead of silently passing; a failed check carries the exact
// error a real run returns, so refusal is identical on both paths.
func readPreconditions(ctx context.Context, pf *preflightCtx, appVerErr, tokenErr error) []appconsume.Precondition {
	credentials := appconsume.Precondition{Name: "credentials_available", Status: appconsume.PreconditionOK}
	if tokenErr != nil {
		credentials.Status = appconsume.PreconditionBlocked
		credentials.Detail = tokenErr.Error()
		credentials.BlockErr = tokenErr
	}

	console := appconsume.Precondition{Name: "console_event_published", Status: appconsume.PreconditionOK}
	switch {
	case len(pf.keyDef.RequiredConsoleEvents) == 0:
		// nothing to verify
	case pf.keyDef.SubscriptionType == eventlib.SubTypeCallback && pf.subscribedCallbacks == nil,
		pf.keyDef.SubscriptionType != eventlib.SubTypeCallback && pf.appVer == nil:
		console.Status = appconsume.PreconditionUnknown
		if appVerErr != nil {
			console.Detail = describeAppMetaErr(appVerErr)
		} else {
			console.Detail = "console ledger unavailable"
		}
	default:
		if err := preflightEventTypes(pf); err != nil {
			console.Status = appconsume.PreconditionBlocked
			console.Detail = err.Error()
			console.BlockErr = err
		}
	}

	scopes := appconsume.Precondition{Name: "scopes_granted", Status: appconsume.PreconditionOK}
	checked, err := preflightScopes(ctx, pf)
	switch {
	case err != nil:
		scopes.Status = appconsume.PreconditionBlocked
		scopes.Detail = err.Error()
		scopes.BlockErr = err
	case !checked:
		// The scope ledger could not be read (no published version for bots,
		// no resolvable token for users). Saying "ok" here would dress up
		// "nobody looked" as "it was verified".
		scopes.Status = appconsume.PreconditionUnknown
		scopes.Detail = "granted scopes could not be read for this identity"
	}

	return []appconsume.Precondition{credentials, console, scopes}
}
