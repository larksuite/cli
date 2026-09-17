// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"errors"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
)

func agentContractAccount() *Account {
	return &Account{
		AppID:           "cli_agent",
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_agent",
	}
}

func TestCheckAccountForAccess(t *testing.T) {
	tests := []struct {
		name    string
		acct    *Account
		spec    AccessSpec
		wantErr bool
	}{
		{
			name: "unclassified profile keeps the pre-existing rules",
			acct: &Account{AppID: "cli_app"},
			spec: AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
		},
		{
			name: "unclassified profile may still act as bot",
			acct: &Account{AppID: "cli_app"},
			spec: AccessSpec{Identity: core.AsBot, AppID: "cli_app"},
		},
		{
			name: "agent profile on the user lane",
			acct: agentContractAccount(),
			spec: AccessSpec{Identity: core.AsUser, AppID: "cli_agent"},
		},
		{
			name:    "agent profile refused on the bot lane",
			acct:    agentContractAccount(),
			spec:    AccessSpec{Identity: core.AsBot, AppID: "cli_agent"},
			wantErr: true,
		},
		{
			name: "agent account with no principal named",
			acct: &Account{AppID: "cli_agent", CredentialKind: core.CredentialAAT},
			spec: AccessSpec{Identity: core.AsUser, AppID: "cli_agent"},
			// A forwarded credential names nobody yet still serves the user lane.
			wantErr: false,
		},
		{
			name: "agent profile restricted to bot only",
			acct: &Account{
				AppID:               "cli_agent",
				CredentialKind:      core.CredentialAAT,
				PrincipalOpenID:     "ou_agent",
				SupportedIdentities: uint8(extcred.SupportsBot),
			},
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_agent"},
			wantErr: true,
		},
		{
			name:    "unknown credential kind fails closed",
			acct:    &Account{AppID: "cli_app", CredentialKind: core.CredentialKind("something-else")},
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			wantErr: true,
		},
		{
			name:    "auto is not a resolvable identity",
			acct:    &Account{AppID: "cli_app"},
			spec:    AccessSpec{Identity: core.AsAuto, AppID: "cli_app"},
			wantErr: true,
		},
		{
			name:    "no account at all",
			acct:    nil,
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkAccountForAccess(tc.acct, tc.spec)
			if tc.wantErr != (err != nil) {
				t.Fatalf("checkAccountForAccess() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestCheckResolvedAccess(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	future := now.Add(time.Hour)
	past := now.Add(-time.Minute)
	userSpec := AccessSpec{Identity: core.AsUser, AppID: "cli_agent"}
	botSpec := AccessSpec{Identity: core.AsBot, AppID: "cli_app"}

	tests := []struct {
		name    string
		acct    *Account
		spec    AccessSpec
		access  *ResolvedAccess
		wantErr bool
	}{
		{
			name:   "agent credential matching its account",
			acct:   agentContractAccount(),
			spec:   userSpec,
			access: &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_agent", ExpiresAt: future},
		},
		{
			name:   "user token on the user lane",
			acct:   &Account{AppID: "cli_app"},
			spec:   AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			access: &ResolvedAccess{Value: "v", Kind: core.CredentialUAT},
		},
		{
			name:   "tenant token on the bot lane",
			acct:   &Account{AppID: "cli_app"},
			spec:   botSpec,
			access: &ResolvedAccess{Value: "v", Kind: core.CredentialTAT},
		},
		{
			name:    "tenant token offered for the user lane",
			acct:    &Account{AppID: "cli_app"},
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialTAT},
			wantErr: true,
		},
		{
			name:    "agent credential offered for the bot lane",
			acct:    agentContractAccount(),
			spec:    botSpec,
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_agent", ExpiresAt: future},
			wantErr: true,
		},
		{
			name:    "resolution contradicts the profile classification",
			acct:    agentContractAccount(),
			spec:    userSpec,
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialUAT},
			wantErr: true,
		},
		{
			name:    "agent credential bound to a different principal",
			acct:    agentContractAccount(),
			spec:    userSpec,
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_other", ExpiresAt: future},
			wantErr: true,
		},
		{
			name:    "agent credential with no principal",
			acct:    agentContractAccount(),
			spec:    userSpec,
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, ExpiresAt: future},
			wantErr: true,
		},
		{
			name:   "forwarded agent credential on an account naming no principal",
			acct:   &Account{AppID: "cli_agent", CredentialKind: core.CredentialAAT},
			spec:   userSpec,
			access: &ResolvedAccess{Value: "v", Kind: core.CredentialAAT},
		},
		{
			name:    "agent credential already expired",
			acct:    agentContractAccount(),
			spec:    userSpec,
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_agent", ExpiresAt: past},
			wantErr: true,
		},
		{
			name: "agent credential with no expiry",
			acct: agentContractAccount(),
			spec: userSpec,
			// Reported separately from "expired": a missing expiry is a provider
			// contract break, and calling it expiry would send an integrator
			// hunting a clock bug.
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_agent"},
			wantErr: true,
		},
		{
			name:    "empty credential",
			acct:    agentContractAccount(),
			spec:    userSpec,
			access:  &ResolvedAccess{Kind: core.CredentialAAT, OpenID: "ou_agent", ExpiresAt: future},
			wantErr: true,
		},
		{
			name: "a uat-classified profile may still serve the bot lane",
			acct: &Account{AppID: "cli_app", CredentialKind: core.CredentialUAT},
			spec: botSpec,
			// uat/tat classify which credential serves the user lane, not the
			// whole account. Requiring equality here would reject every bot call
			// on such a profile.
			access: &ResolvedAccess{Value: "v", Kind: core.CredentialTAT},
		},
		{
			name:   "a tat-classified profile may still serve the user lane",
			acct:   &Account{AppID: "cli_app", CredentialKind: core.CredentialTAT},
			spec:   AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			access: &ResolvedAccess{Value: "v", Kind: core.CredentialUAT},
		},
		{
			name:    "an agent credential must not surface on an unclassified profile",
			acct:    &Account{AppID: "cli_app"},
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_x", ExpiresAt: future},
			wantErr: true,
		},
		{
			name:    "unknown credential kind",
			acct:    &Account{AppID: "cli_app"},
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			access:  &ResolvedAccess{Value: "v", Kind: core.CredentialKind("mystery")},
			wantErr: true,
		},
		{
			name:    "no access at all",
			acct:    &Account{AppID: "cli_app"},
			spec:    AccessSpec{Identity: core.AsUser, AppID: "cli_app"},
			access:  nil,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkResolvedAccess(tc.acct, tc.spec, tc.access, now)
			if tc.wantErr != (err != nil) {
				t.Fatalf("checkResolvedAccess() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestAgentExpiryFailuresAreDistinguishable pins that "no expiry reported" and
// "already expired" are different errors. Collapsing them would tell an
// integrator whose provider simply omits the field that their clock or TTL is
// wrong.
func TestAgentExpiryFailuresAreDistinguishable(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	acct := agentContractAccount()
	spec := AccessSpec{Identity: core.AsUser, AppID: "cli_agent"}
	base := ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_agent"}

	missing := base
	errMissing := checkResolvedAccess(acct, spec, &missing, now)
	var internalErr *errs.InternalError
	if !errors.As(errMissing, &internalErr) {
		t.Fatalf("no-expiry error = %T %v, want *errs.InternalError", errMissing, errMissing)
	}

	stale := base
	stale.ExpiresAt = now.Add(-time.Minute)
	errStale := checkResolvedAccess(acct, spec, &stale, now)
	var authErr *errs.AuthenticationError
	if !errors.As(errStale, &authErr) || authErr.Subtype != errs.SubtypeTokenExpired {
		t.Fatalf("expired error = %T %v, want token_expired", errStale, errStale)
	}
}

// TestUnknownIdentityIsReportedAsBadInput pins that an unrecognized --as value
// is a validation error naming the flag, not an internal one: the cause is a
// user typo, and the message should say so.
func TestUnknownIdentityIsReportedAsBadInput(t *testing.T) {
	err := checkAccountForAccess(&Account{AppID: "cli_app"}, AccessSpec{Identity: "bogus", AppID: "cli_app"})
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("error = %T %v, want invalid_argument", err, err)
	}
	if validationErr.Param != "--as" {
		t.Fatalf("param = %q, want --as", validationErr.Param)
	}
}

// TestContractChecksAreOrderIndependent pins that the checks carry no state
// between calls. If they did, a command could pass only because an earlier one
// in the same process had already resolved something.
func TestContractChecksAreOrderIndependent(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	acct := agentContractAccount()
	good := &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_agent", ExpiresAt: now.Add(time.Hour)}
	bad := &ResolvedAccess{Value: "v", Kind: core.CredentialAAT, OpenID: "ou_other", ExpiresAt: now.Add(time.Hour)}
	spec := AccessSpec{Identity: core.AsUser, AppID: "cli_agent"}

	for i := range 3 {
		if err := checkResolvedAccess(acct, spec, good, now); err != nil {
			t.Fatalf("round %d: valid access rejected: %v", i, err)
		}
		if err := checkResolvedAccess(acct, spec, bad, now); err == nil {
			t.Fatalf("round %d: mismatched principal accepted", i)
		}
	}
}
