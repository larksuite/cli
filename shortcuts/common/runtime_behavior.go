// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/runtimeplan"
)

// RuntimeCapability identifies a source-neutral execution surface required by
// a shortcut. It deliberately contains no credential-mode or edition detail.
type RuntimeCapability string

const (
	// RuntimeCapabilityRealtimeEvents covers long-lived event/WebSocket
	// consumers.
	RuntimeCapabilityRealtimeEvents RuntimeCapability = "realtime_events"
)

// LocalIntrospection runs before runtime capability, identity, Profile,
// credential, or client resolution. Returning handled=true completes the
// invocation with out written to stdout.
type LocalIntrospection func(cmd *cobra.Command) (out []byte, handled bool, err error)

// RuntimeMountOption configures source-neutral runtime behavior on a shortcut's
// existing PostMount seam. Its implementation is sealed so new behavior can be
// added without changing Shortcut's stable exported field layout.
type RuntimeMountOption interface {
	apply(*runtimeMountConfig)
}

type runtimeMountOptionFunc func(*runtimeMountConfig)

func (f runtimeMountOptionFunc) apply(config *runtimeMountConfig) {
	f(config)
}

type runtimeMountConfig struct {
	capabilities       []RuntimeCapability
	localIntrospection LocalIntrospection
}

type runtimeBehaviorContextKey struct{}

// RequireRuntimeCapabilities declares the invocation surfaces required by a
// shortcut. The runtime plan checks them before credential or client setup.
func RequireRuntimeCapabilities(capabilities ...RuntimeCapability) RuntimeMountOption {
	return runtimeMountOptionFunc(func(config *runtimeMountConfig) {
		config.capabilities = append(config.capabilities, capabilities...)
	})
}

// EnableLocalIntrospection installs a read-only shortcut path that can finish
// without initializing the active runtime.
func EnableLocalIntrospection(hook LocalIntrospection) RuntimeMountOption {
	return runtimeMountOptionFunc(func(config *runtimeMountConfig) {
		config.localIntrospection = hook
	})
}

// RuntimePostMount builds a PostMount hook for source-neutral runtime behavior.
// Runtime state is carried in the command invocation context, not in a global
// registry, so separately built command trees cannot affect one another.
func RuntimePostMount(options ...RuntimeMountOption) func(*cobra.Command) {
	config := runtimeMountConfig{}
	for _, option := range options {
		if option != nil {
			option.apply(&config)
		}
	}
	capabilities := append([]RuntimeCapability(nil), config.capabilities...)
	localIntrospection := config.localIntrospection

	return func(cmd *cobra.Command) {
		if cmd == nil {
			return
		}
		runtimeCapabilities := make([]runtimeplan.Capability, 0, len(capabilities))
		for _, capability := range capabilities {
			if capability != "" {
				runtimeCapabilities = append(runtimeCapabilities, runtimeplan.Capability(capability))
			}
		}
		cmdutil.SetRuntimeCapabilities(cmd, runtimeCapabilities...)

		if localIntrospection == nil || cmd.RunE == nil {
			return
		}
		runE := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			ctx := context.WithValue(cmd.Context(), runtimeBehaviorContextKey{}, localIntrospection)
			cmd.SetContext(ctx)
			return runE(cmd, args)
		}
	}
}

func localIntrospectionFromContext(ctx context.Context) LocalIntrospection {
	if ctx == nil {
		return nil
	}
	hook, _ := ctx.Value(runtimeBehaviorContextKey{}).(LocalIntrospection)
	return hook
}
