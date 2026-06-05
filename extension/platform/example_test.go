// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/extension/platform"
)

// ExampleNewPlugin_observer registers an audit Observer that fires
// after every command, regardless of success or failure.
func ExampleNewPlugin_observer() {
	p, _ := platform.NewPlugin("audit", "0.1.0").
		Observer(platform.After, "log", platform.All(),
			func(ctx context.Context, inv platform.Invocation) {
				_ = inv.Cmd().Path() // do something useful with the command
			}).
		FailOpen().
		Build()
	fmt.Println(p.Name(), p.Version())
	// Output: audit 0.1.0
}

// ExampleNewPlugin_wrapper registers a Wrap that short-circuits any
// write-class command. The framework converts the returned
// *AbortError into a structured "hook" envelope; observers still
// fire on the After stage so audit sees the attempt.
func ExampleNewPlugin_wrapper() {
	p, _ := platform.NewPlugin("policy-plugin", "0.1.0").
		Wrap("block-writes", platform.ByWrite(),
			func(next platform.Handler) platform.Handler {
				return func(ctx context.Context, inv platform.Invocation) error {
					return &platform.AbortError{
						HookName: "block-writes",
						Reason:   "writes are disabled for this session",
					}
				}
			}).
		FailOpen().
		Build()
	fmt.Println(p.Capabilities().FailurePolicy == platform.FailOpen)
	// Output: true
}

// ExampleNewPlugin_restrict registers a policy plugin that allows
// only docs/* read commands. Note that Restrict() implicitly sets
// FailClosed — a policy plugin must abort the binary if it fails to
// install, not silently disappear.
func ExampleNewPlugin_restrict() {
	p, _ := platform.NewPlugin("readonly-docs", "0.1.0").
		Restrict(&platform.Rule{
			Name:    "docs-only",
			Allow:   []string{"docs/**"},
			MaxRisk: platform.RiskRead,
		}).
		Build()
	caps := p.Capabilities()
	fmt.Println(caps.Restricts, caps.FailurePolicy == platform.FailClosed)
	// Output: true true
}
