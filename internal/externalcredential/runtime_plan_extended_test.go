// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package externalcredential

import (
	"context"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
	localtransport "github.com/larksuite/cli/internal/transport"
)

type runtimePlanTestProvider struct{ name string }

func (p runtimePlanTestProvider) Name() string { return p.name }
func (p runtimePlanTestProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, nil
}
func (p runtimePlanTestProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

type runtimePlanTransportProvider struct{}

func (runtimePlanTransportProvider) Name() string { return "test-transport" }
func (runtimePlanTransportProvider) ResolveInterceptor(context.Context) exttransport.Interceptor {
	return nil
}

func registerRuntimePlanTransportProvider(t *testing.T, provider exttransport.Provider) {
	t.Helper()
	original := exttransport.GetProvider()
	exttransport.Register(provider)
	t.Cleanup(func() { exttransport.Register(original) })
}

func stubLocalProxyTransportConfig(t *testing.T, config *localtransport.Config) {
	t.Helper()
	original := loadLocalProxyTransportConfig
	loadLocalProxyTransportConfig = func() (*localtransport.Config, error) {
		return config, nil
	}
	t.Cleanup(func() { loadLocalProxyTransportConfig = original })
}

func TestRuntimePlanAllowsBuiltinEnvironmentProviderReplacement(t *testing.T) {
	if err := validateRegisteredCredentialProviders([]extcred.Provider{
		runtimePlanTestProvider{name: "env"},
	}); err != nil {
		t.Fatalf("environment provider validation = %v", err)
	}
}

func TestRuntimePlanRejectsCompileTimeCredentialProvider(t *testing.T) {
	if err := validateRegisteredCredentialProviders([]extcred.Provider{
		runtimePlanTestProvider{name: "custom-vault"},
	}); err == nil {
		t.Fatal("expected custom provider conflict")
	}
}

func TestDirectRuntimePlanRetainsLocalTransportCustomizations(t *testing.T) {
	registerRuntimePlanTransportProvider(t, runtimePlanTransportProvider{})
	stubLocalProxyTransportConfig(t, &localtransport.Config{Enable: true})

	config := &Config{
		Version: 1,
		Mode:    ModeDirect,
	}
	app := &core.AppConfig{
		AppId: "cli_test",
		Brand: core.BrandFeishu,
	}
	base := http.DefaultTransport
	plan := newRuntimePlan(app, config)
	got, err := plan.Wrap(base)
	if err != nil {
		t.Fatalf("direct runtime rejected ordinary local transport customizations: %v", err)
	}
	if got != base {
		t.Fatalf("direct runtime wrapped base transport: got %T %p, want %T %p", got, got, base, base)
	}
}

func TestProxyRuntimePlanRejectsCompileTimeTransportExtension(t *testing.T) {
	registerRuntimePlanTransportProvider(t, runtimePlanTransportProvider{})
	for _, mode := range []Mode{ModeCredentialProxy, ModePlatformProxy} {
		t.Run(string(mode), func(t *testing.T) {
			config := &Config{
				Version:        1,
				Mode:           mode,
				RemoteEndpoint: "https://credentials.example",
			}
			app := &core.AppConfig{AppId: "cli_test", Brand: core.BrandFeishu}
			plan := newRuntimePlan(app, config)
			if _, err := plan.Wrap(http.DefaultTransport); err == nil {
				t.Fatal("expected proxy mode and compile-time Transport extension to be rejected")
			}
		})
	}
}

func TestProxyRuntimePlanRejectsEnabledLocalProxyTransport(t *testing.T) {
	registerRuntimePlanTransportProvider(t, nil)
	stubLocalProxyTransportConfig(t, &localtransport.Config{Enable: true})
	for _, mode := range []Mode{ModeCredentialProxy, ModePlatformProxy} {
		t.Run(string(mode), func(t *testing.T) {
			config := &Config{
				Version:        1,
				Mode:           mode,
				RemoteEndpoint: "https://credentials.example",
			}
			app := &core.AppConfig{AppId: "cli_test", Brand: core.BrandFeishu}
			plan := newRuntimePlan(app, config)
			if _, err := plan.Wrap(http.DefaultTransport); err == nil {
				t.Fatal("expected proxy mode and enabled local proxy transport to be rejected")
			}
		})
	}
}

func TestManagedRuntimeRejectsUnreviewedCapabilities(t *testing.T) {
	err := managedCapabilityPolicy(runtimeplan.Capability("future_data_plane"))
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("capability denial is not typed: %v", err)
	}
	if problem.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("capability denial subtype = %q, want %q", problem.Subtype, errs.SubtypeFailedPrecondition)
	}
}

func TestManagedRuntimeCapabilityDenialsDistinguishCredentialAndProfileOwnership(t *testing.T) {
	tests := []struct {
		name       string
		capability runtimeplan.Capability
		message    string
		hint       string
	}{
		{
			name:       "credential management",
			capability: runtimeplan.CapabilityLocalCredentialManagement,
			message:    "local credential management is unavailable while credentials are owned by the active runtime",
			hint:       "manage authorization through the configured credential platform",
		},
		{
			name:       "Profile and identity changes",
			capability: runtimeplan.CapabilityLocalProfileMutation,
			message:    "local Profile and identity changes are unavailable while the active runtime uses a deployment-managed Profile",
			hint:       "ask the deploying integrator to update the secretless Profile selector or identity policy in config.json",
		},
		{
			name:       "real-time events",
			capability: runtimeplan.CapabilityRealtimeEvents,
			message:    "real-time event consumption is not supported by the active credential runtime",
			hint:       "use a deployment configured with local credentials for event consumption; managed event support is unavailable in this version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := managedCapabilityPolicy(tt.capability)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("capability denial is not typed: %T %v", err, err)
			}
			if problem.Category != errs.CategoryValidation ||
				problem.Subtype != errs.SubtypeFailedPrecondition ||
				problem.Message != tt.message ||
				problem.Hint != tt.hint {
				t.Fatalf("capability denial = %#v, want validation/failed_precondition message %q hint %q",
					problem, tt.message, tt.hint)
			}
		})
	}
}
