// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package runtimeplan defines the source-neutral capabilities selected once
// for a CLI invocation. Concrete credential systems implement these seams;
// commands and business shortcuts must not inspect their configuration.
package runtimeplan

import (
	"net/http"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
)

// Capability identifies an execution surface whose availability is selected
// during bootstrap.
type Capability string

const (
	// CapabilityLocalCredentialManagement covers commands that mutate
	// credentials owned by the local Profile/keychain.
	CapabilityLocalCredentialManagement Capability = "local_credential_management"

	// CapabilityLocalProfileMutation covers changes to the local Profile file
	// and active Profile selection. It is separate from credential-provider
	// ownership so ordinary environment/extension providers retain the
	// established ability to prepare local Profiles for later invocations.
	CapabilityLocalProfileMutation Capability = "local_profile_mutation"

	// CapabilityRealtimeEvents covers long-lived event/WebSocket consumers.
	CapabilityRealtimeEvents Capability = "realtime_events"
)

// CapabilityPolicy decides whether one source-neutral execution surface is
// available. A nil policy preserves the ordinary CLI behavior and allows all
// capabilities.
type CapabilityPolicy func(Capability) error

// MetadataPolicy controls whether startup may consult the remote API catalog.
// Its zero value deliberately preserves the ordinary CLI behavior.
type MetadataPolicy uint8

const (
	MetadataRemoteAllowed MetadataPolicy = iota
	MetadataEmbeddedOnly
)

// TransportWrapper installs a data-plane policy around an existing transport.
// A nil wrapper preserves the existing transport exactly.
type TransportWrapper func(http.RoundTripper) (http.RoundTripper, error)

// RemoteFilePolicy validates service-returned file references and declares
// whether transfers must remain inside the runtime-managed data plane.
type RemoteFilePolicy interface {
	ValidateRemoteFile(rawURL string) error
	UsesManagedFilePlane() bool
}

// Description is sanitized runtime metadata for diagnostics. It contains no
// credential material and is not used to make security decisions.
type Description struct {
	Managed           bool
	Variant           string
	ProxiesRequests   bool
	DataPlaneEndpoint string
}

// Options constructs an invocation plan. Callers should provide only
// source-neutral capabilities; product configuration remains private to the
// concrete adapter captured by these interfaces and functions.
type Options struct {
	CredentialProvider         extcred.Provider
	ReplaceCredentialProviders bool
	WrapTransport              TransportWrapper
	RemoteFiles                RemoteFilePolicy
	Capabilities               CapabilityPolicy
	Metadata                   MetadataPolicy
	Description                Description
	StartupError               error
}

// Plan is an immutable invocation-scoped composition of runtime capabilities.
// Its collaborators may maintain their own credential caches, but the selected
// policy and dependency graph cannot be changed after construction.
type Plan struct {
	credentialProvider         extcred.Provider
	replaceCredentialProviders bool
	wrapTransport              TransportWrapper
	remoteFiles                RemoteFilePolicy
	capabilities               CapabilityPolicy
	metadata                   MetadataPolicy
	description                Description
	startupError               error
}

// Default returns the ordinary CLI plan. It deliberately has no wrapper or
// credential override, so Standard behavior stays on the established path.
func Default() *Plan {
	return New(Options{})
}

// Failed returns a fail-closed plan. It is useful when bootstrap discovers a
// policy it cannot safely activate but command construction must still finish
// so the normal typed error envelope can be emitted.
func Failed(err error, metadata MetadataPolicy) *Plan {
	return New(Options{
		StartupError: err,
		Metadata:     metadata,
	})
}

// New constructs an immutable Plan.
func New(opts Options) *Plan {
	return &Plan{
		credentialProvider:         opts.CredentialProvider,
		replaceCredentialProviders: opts.ReplaceCredentialProviders,
		wrapTransport:              opts.WrapTransport,
		remoteFiles:                opts.RemoteFiles,
		capabilities:               opts.Capabilities,
		metadata:                   opts.Metadata,
		description:                opts.Description,
		startupError:               opts.StartupError,
	}
}

// Ensure normalizes a nil plan to the ordinary CLI behavior.
func Ensure(plan *Plan) *Plan {
	if plan == nil {
		return Default()
	}
	return plan
}

// StartupError reports a bootstrap failure that blocks credential resolution
// and every managed data-plane boundary. Purely local recovery and
// introspection commands may remain available without touching the plan.
func (p *Plan) StartupError() error {
	if p == nil {
		return nil
	}
	return p.startupError
}

// CredentialProvider returns an invocation-selected provider and whether it
// replaces the registered provider chain.
func (p *Plan) CredentialProvider() (extcred.Provider, bool) {
	if p == nil {
		return nil, false
	}
	return p.credentialProvider, p.replaceCredentialProviders
}

// Wrap applies the selected data-plane policy.
func (p *Plan) Wrap(base http.RoundTripper) (http.RoundTripper, error) {
	if p == nil {
		return base, nil
	}
	if p.startupError != nil {
		return nil, p.startupError
	}
	if p.wrapTransport == nil {
		return base, nil
	}
	return p.wrapTransport(base)
}

// ValidateRemoteFile applies the selected file-reference policy.
func (p *Plan) ValidateRemoteFile(rawURL string) error {
	if p == nil {
		return nil
	}
	if p.startupError != nil {
		return p.startupError
	}
	if p.remoteFiles == nil {
		return nil
	}
	return p.remoteFiles.ValidateRemoteFile(rawURL)
}

// UsesManagedFilePlane reports whether file transfers must use the configured
// runtime HTTP client.
func (p *Plan) UsesManagedFilePlane() bool {
	return p != nil && p.remoteFiles != nil && p.remoteFiles.UsesManagedFilePlane()
}

// Require checks whether an execution capability is available.
func (p *Plan) Require(capability Capability) error {
	if p == nil {
		p = Default()
	}
	if p.startupError != nil {
		return p.startupError
	}
	switch capability {
	case CapabilityLocalCredentialManagement, CapabilityLocalProfileMutation, CapabilityRealtimeEvents:
		// Known source-neutral capability.
	default:
		return errs.NewInternalError(errs.SubtypeUnknown,
			"runtime plan contains unrecognized capability %q", capability)
	}
	if p.capabilities == nil {
		return nil
	}
	return p.capabilities(capability)
}

// AllowsRemoteMetadata reports whether startup may consult the remote API
// catalog. Proxy runtimes disable it to prevent pre-policy network egress.
func (p *Plan) AllowsRemoteMetadata() bool {
	if p == nil {
		return true
	}
	switch p.metadata {
	case MetadataRemoteAllowed:
		return true
	case MetadataEmbeddedOnly:
		return false
	default:
		// An invalid policy value must never open a pre-policy network path.
		return false
	}
}

// Describe returns sanitized diagnostic metadata by value.
func (p *Plan) Describe() Description {
	if p == nil {
		return Description{}
	}
	return p.description
}
