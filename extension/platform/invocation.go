// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "time"

// Invocation is the per-command data a Wrapper / Observer receives. It
// is a read-only interface: the framework implementation lives in
// internal/hook and is never visible to plugins, so plugin code cannot
// mutate denial state.
//
// The interface is deliberately NOT a context.Context — it is data only,
// no cancellation. ctx (from the handler signature) carries
// cancellation / timeout / trace propagation.
//
// Accessor semantics:
//
//   - Cmd / Args / Started are populated before the first hook fires
//   - Err is populated for After observers and the post-next portion of
//     a Wrapper (the value the wrapped handler returned)
//   - DeniedByPolicy / DenialLayer / DenialPolicySource are populated by
//     the framework's denial guard before any hook runs
type Invocation interface {
	// Cmd returns the read-only metadata view of the dispatched command.
	Cmd() CommandView

	// Args returns a fresh copy of the positional args.
	Args() []string

	// Started is the wall-clock time the outermost RunE wrapper began.
	Started() time.Time

	// Err is the error the wrapped handler returned. Populated for
	// After observers and the post-next portion of a Wrapper. nil
	// before the handler runs.
	Err() error

	// DeniedByPolicy reports whether the command was rejected by either
	// strict-mode or user-layer policy before the chain reached the
	// hook. Observers fire even for denied commands (audit case); Wrap
	// is physically isolated by the framework so plugins do not need
	// to check this themselves before calling next.
	DeniedByPolicy() bool

	// DenialLayer returns the layer that rejected the command:
	//
	//   ""             - not denied
	//   "strict_mode"  - credential strict-mode
	//   "policy"       - user-layer Rule (Plugin.Restrict() or yaml)
	DenialLayer() string

	// DenialPolicySource returns the specific source identifier
	// ("plugin:secaudit", "yaml", "strict-mode"). Empty when not denied.
	DenialPolicySource() string
}
