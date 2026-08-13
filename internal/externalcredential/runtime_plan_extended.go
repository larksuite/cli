// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"net/http"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
	localtransport "github.com/larksuite/cli/internal/transport"
)

var loadLocalProxyTransportConfig = localtransport.Load

type remoteFilePolicy struct {
	config *Config
}

func (p remoteFilePolicy) ValidateRemoteFile(rawURL string) error {
	return ValidateFileURL(rawURL, p.config)
}

func (p remoteFilePolicy) UsesManagedFilePlane() bool {
	return p.config != nil && p.config.Mode.IsProxy()
}

func newRuntimePlan(app *core.AppConfig, config *Config) *runtimeplan.Plan {
	if err := validateRegisteredCredentialProviders(extcred.Providers()); err != nil {
		return runtimeplan.Failed(err, runtimeplan.MetadataEmbeddedOnly)
	}
	provider := NewProvider(app, config)
	proxy := config != nil && config.Mode.IsProxy()

	var wrap runtimeplan.TransportWrapper
	if proxy {
		wrap = func(base http.RoundTripper) (http.RoundTripper, error) {
			if exttransport.GetProvider() != nil {
				return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
					"external credential proxy mode cannot be combined with a compile-time transport extension").
					WithHint("remove the compile-time transport extension when using external-credential.json")
			}
			localProxy, err := loadLocalProxyTransportConfig()
			if err != nil {
				return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
					"external credential proxy mode cannot use the configured local proxy transport: %v", err).
					WithCause(err)
			}
			if localProxy != nil && localProxy.Enabled() {
				return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
					"external credential proxy mode cannot be combined with proxy_config.json").
					WithHint("disable the local proxy plugin before using external-credential.json")
			}
			return WrapTransport(base, TransportOptions{
				Mode:       config.Mode,
				Endpoint:   config.RemoteEndpoint,
				AppID:      app.AppId,
				Brand:      app.Brand,
				Credential: provider,
			})
		}
	}

	var files runtimeplan.RemoteFilePolicy
	if proxy {
		files = remoteFilePolicy{config: config}
	}
	metadata := runtimeplan.MetadataRemoteAllowed
	if proxy {
		metadata = runtimeplan.MetadataEmbeddedOnly
	}
	return runtimeplan.New(runtimeplan.Options{
		CredentialProvider:         provider,
		ReplaceCredentialProviders: true,
		WrapTransport:              wrap,
		RemoteFiles:                files,
		Capabilities:               managedCapabilityPolicy,
		Metadata:                   metadata,
		Description: runtimeplan.Description{
			Managed:           true,
			Variant:           string(config.Mode),
			ProxiesRequests:   proxy,
			DataPlaneEndpoint: config.RemoteEndpoint,
		},
	})
}

func managedCapabilityPolicy(capability runtimeplan.Capability) error {
	switch capability {
	case runtimeplan.CapabilityRealtimeEvents:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"real-time event consumption is not supported by the active credential runtime").
			WithHint("use a deployment configured with local credentials for event consumption; managed event support is unavailable in this version")
	case runtimeplan.CapabilityLocalCredentialManagement:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"local credential management is unavailable while credentials are owned by the active runtime").
			WithHint("manage authorization through the configured credential platform")
	case runtimeplan.CapabilityLocalProfileMutation:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"local Profile and identity changes are unavailable while the active runtime uses a deployment-managed Profile").
			WithHint("ask the deploying integrator to update the secretless Profile selector or identity policy in config.json")
	default:
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"runtime capability %q has not been enabled by the active credential runtime", capability).
			WithHint("use a deployment whose credential runtime explicitly supports this capability")
	}
}

func validateRegisteredCredentialProviders(registered []extcred.Provider) error {
	for _, provider := range registered {
		// The built-in environment provider is deliberately replaced. Any
		// other compile-time source is an ambiguous authority and fails closed.
		if provider != nil && provider.Name() != "env" {
			err := errs.NewConfigError(errs.SubtypeInvalidConfig,
				"system external credential mode cannot be combined with compile-time credential provider %q", provider.Name()).
				WithHint("remove the credential extension from the Extended binary")
			return err
		}
	}
	return nil
}
