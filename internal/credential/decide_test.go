// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"testing"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// stubDecideProvider satisfies extcred.Provider for building providerAccount
// literals; decideIdentity only ever calls Name() on it.
type stubDecideProvider struct{ name string }

func (s stubDecideProvider) Name() string  { return s.name }
func (s stubDecideProvider) Priority() int { return 0 }
func (s stubDecideProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, nil
}
func (s stubDecideProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func pa(providerName, appID string) *providerAccount {
	return &providerAccount{
		acct:   &Account{AppID: appID},
		source: extensionTokenSource{provider: stubDecideProvider{name: providerName}},
	}
}

func appIDOnlyBlock(appID string) *extcred.BlockError {
	return &extcred.BlockError{
		Provider:      "env",
		Reason:        envvars.CliAppID + " is set but no app secret or access token is available",
		Code:          extcred.BlockReasonCredentialIncomplete,
		RequiredAnyOf: []string{envvars.CliAppSecret, envvars.CliUserAccessToken, envvars.CliTenantAccessToken},
		PresentKeys:   []string{envvars.CliAppID},
		AppID:         appID,
	}
}

func uatOnlyBlock() *extcred.BlockError {
	return &extcred.BlockError{
		Provider:    "env",
		Reason:      envvars.CliUserAccessToken + " is set but " + envvars.CliAppID + " is missing",
		Code:        extcred.BlockReasonCredentialIncomplete,
		MissingKeys: []string{envvars.CliAppID},
		PresentKeys: []string{envvars.CliUserAccessToken},
	}
}

// TestDecideIdentity exercises the selection matrix as data: decideIdentity is
// pure, so every rule (precedence, conflict detection, error attribution) is
// table-testable without env vars or config fixtures.
func TestDecideIdentity(t *testing.T) {
	tenantA := &core.MultiAppConfig{
		CurrentApp: "tenant_a",
		Apps:       []core.AppConfig{{Name: "tenant_a", AppId: "cli_a"}},
	}
	noCurrent := &core.MultiAppConfig{
		Apps: []core.AppConfig{{Name: "tenant_a", AppId: "cli_a"}},
	}
	invalidConfigErr := errs.NewConfigError(errs.SubtypeInvalidConfig, "invalid config format")
	notConfiguredErr := core.NotConfiguredError()

	cases := []struct {
		name    string
		in      identityInputs
		route   credentialRoute
		source  CredentialSourceKind
		matched bool
		subtype errs.Subtype // "" = success expected
	}{
		{
			name:   "managed provider wins over explicit profile",
			in:     identityInputs{profile: "tenant_a", profileSrc: SourceFlagProfile, managed: pa("sidecar", "sidecar_app"), config: tenantA},
			route:  routeManaged,
			source: SourceExtension("sidecar"),
		},
		{
			name:    "profile conflicts with complete direct env app_id",
			in:      identityInputs{profile: "tenant_a", profileSrc: SourceFlagProfile, direct: pa("env", "cli_x"), directKeys: []string{envvars.CliAppID, envvars.CliAppSecret}, config: tenantA},
			subtype: errs.SubtypeProfileAppCredentialConflict,
		},
		{
			name:    "matched complete direct env yields profile route",
			in:      identityInputs{profile: "tenant_a", profileSrc: SourceEnvProfile, direct: pa("env", "cli_a"), directKeys: []string{envvars.CliAppID, envvars.CliAppSecret}, config: tenantA},
			route:   routeProfile,
			source:  SourceEnvProfile,
			matched: true,
		},
		{
			name:    "APP_ID-only block matching the profile yields profile route",
			in:      identityInputs{profile: "tenant_a", profileSrc: SourceFlagProfile, directBlock: appIDOnlyBlock("cli_a"), directKeys: []string{envvars.CliAppID}, config: tenantA},
			route:   routeProfile,
			source:  SourceFlagProfile,
			matched: true,
		},
		{
			name:    "APP_ID-only block mismatching the profile is a hard conflict",
			in:      identityInputs{profile: "tenant_a", profileSrc: SourceFlagProfile, directBlock: appIDOnlyBlock("cli_x"), directKeys: []string{envvars.CliAppID}, config: tenantA},
			subtype: errs.SubtypeProfileAppCredentialConflict,
		},
		{
			name:    "UAT-only block with a valid profile keeps the repair error",
			in:      identityInputs{profile: "tenant_a", profileSrc: SourceFlagProfile, directBlock: uatOnlyBlock(), config: tenantA},
			subtype: errs.SubtypeAppCredentialIncomplete,
		},
		{
			name:    "block without profile is app_credential_incomplete",
			in:      identityInputs{directBlock: appIDOnlyBlock("cli_a"), directKeys: []string{envvars.CliAppID}},
			subtype: errs.SubtypeAppCredentialIncomplete,
		},
		{
			name:   "complete direct env without profile wins",
			in:     identityInputs{direct: pa("env", "cli_env"), directKeys: []string{envvars.CliAppID, envvars.CliAppSecret}},
			route:  routeDirectEnv,
			source: SourceEnvAppID,
		},
		{
			name:    "malformed config is not masked as profile_not_found",
			in:      identityInputs{profile: "tenant_a", profileSrc: SourceFlagProfile, configErr: invalidConfigErr},
			subtype: errs.SubtypeInvalidConfig,
		},
		{
			name:    "absent config degrades to profile_not_found",
			in:      identityInputs{profile: "ghost", profileSrc: SourceEnvProfile, configErr: notConfiguredErr},
			subtype: errs.SubtypeProfileNotFound,
		},
		{
			name:    "profile missing from a valid config is profile_not_found even with incomplete env",
			in:      identityInputs{profile: "ghost", profileSrc: SourceEnvProfile, directBlock: appIDOnlyBlock("cli_a"), directKeys: []string{envvars.CliAppID}, config: tenantA},
			subtype: errs.SubtypeProfileNotFound,
		},
		{
			name:   "config default reports currentApp",
			in:     identityInputs{config: tenantA},
			route:  routeConfigDefault,
			source: SourceConfigCurrentApp,
		},
		{
			name:   "config default without currentApp reports firstApp",
			in:     identityInputs{config: noCurrent},
			route:  routeConfigDefault,
			source: SourceConfigFirstApp,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := decideIdentity(tc.in)
			if tc.subtype != "" {
				if err == nil {
					t.Fatalf("decideIdentity = %+v, want error subtype %q", d, tc.subtype)
				}
				prob, ok := errs.ProblemOf(err)
				if !ok || prob.Subtype != tc.subtype {
					t.Fatalf("error = %v, want subtype %q", err, tc.subtype)
				}
				return
			}
			if err != nil {
				t.Fatalf("decideIdentity: %v", err)
			}
			if d.route != tc.route {
				t.Errorf("route = %d, want %d", d.route, tc.route)
			}
			if d.selection.Source != tc.source {
				t.Errorf("source = %q, want %q", d.selection.Source, tc.source)
			}
			if d.selection.DirectCredentialEnv.Matched != tc.matched {
				t.Errorf("matched = %v, want %v", d.selection.DirectCredentialEnv.Matched, tc.matched)
			}
		})
	}
}
