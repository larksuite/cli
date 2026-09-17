// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
)

// This file is the single owner of the account/access invariants. Every rule
// here is a pure function of its arguments: nothing is remembered between
// calls, so a command can never succeed only because an earlier command in the
// same process already resolved something. Keeping the rules in one place is
// what makes "an Agent Employee credential never reaches a bot request" a
// property of the code rather than a habit each call site has to repeat.

// identityForKind reports which caller identity a credential kind may serve.
// User access tokens and Agent Employee tokens both travel the user lane;
// tenant tokens travel the bot lane. There is no third lane.
func identityForKind(kind core.CredentialKind) (core.Identity, bool) {
	switch kind {
	case core.CredentialUAT, core.CredentialAAT:
		return core.AsUser, true
	case core.CredentialTAT:
		return core.AsBot, true
	default:
		return "", false
	}
}

// accountSupportsBotOnly reports whether the account declared bot as its only
// usable identity.
func accountSupportsBotOnly(acct *Account) bool {
	if acct == nil || acct.SupportedIdentities == 0 {
		return false
	}
	return extcred.IdentitySupport(acct.SupportedIdentities).BotOnly()
}

// checkAccountForAccess is the pre-resolution gate. It runs before any network
// call or keychain read, so a request that can never be satisfied fails without
// minting anything or touching stored credentials.
func checkAccountForAccess(acct *Account, spec AccessSpec) error {
	if acct == nil {
		return core.NotConfiguredError()
	}
	if acct.AppID == "" {
		return errs.NewConfigError(errs.SubtypeNotConfigured,
			"no app is configured for the current profile")
	}
	switch spec.Identity {
	case core.AsUser, core.AsBot:
	case core.AsAuto:
		return errs.NewInternalError(errs.SubtypeSDKError,
			"access resolution requires a concrete identity, got auto")
	default:
		// An unrecognized value reaches here when a command does not restrict
		// --as. Rejecting it as a bad flag rather than as a broken response
		// points the user at their own typo instead of at the CLI.
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown identity %q", spec.Identity).
			WithParam("--as").
			WithHint("use --as user, --as bot, or --as auto")
	}

	switch acct.CredentialKind {
	case "":
		// Unclassified profile: the pre-existing user/bot rules apply untouched.
		return nil
	case core.CredentialUAT, core.CredentialTAT:
		return nil
	case core.CredentialAAT:
		return checkAgentAccountForAccess(acct, spec)
	default:
		return errs.NewConfigError(errs.SubtypeInvalidConfig,
			"profile declares an unsupported credential kind %q", acct.CredentialKind).
			WithHint("Rerun `lark-cli config init` to reclassify this profile.")
	}
}

// checkAgentAccountForAccess holds the rules specific to an Agent Employee
// account: it may only be used on the user lane. A named principal is not
// required here; a forwarded credential has none, and a bound profile missing
// one is caught by the resolver that mints against it.
func checkAgentAccountForAccess(acct *Account, spec AccessSpec) error {
	if accountSupportsBotOnly(acct) {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"this profile is restricted to bot identity but its credentials belong to an Agent Employee").
			WithHint("Remove the bot-only restriction from the profile, or configure a separate profile for bot calls.")
	}
	if spec.Identity == core.AsBot {
		return agentBotIdentityError()
	}
	return nil
}

// AgentBotIdentityError is the shared refusal for running an Agent Employee
// profile as a bot. Identity selection needs the same wording as credential
// resolution so a user sees one explanation regardless of where it is caught.
func AgentBotIdentityError() error { return agentBotIdentityError() }

// agentBotIdentityError is the single wording for "an Agent Employee cannot act
// as a bot". The same app credentials mint an Agent Employee token rather than
// a tenant token, so falling back to bot would put an Agent Employee credential
// on the tenant wire — a request that succeeds while meaning something else.
func agentBotIdentityError() error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"Agent Employee profiles cannot use bot identity").
		WithHint("Use `--as user` or `--as auto`. If commands fail this way without passing --as, the profile itself selects bot: change it with `lark-cli config default-as user`, or check `lark-cli config strict-mode`.")
}

// AgentLegacyTokenError reports that a request reached the pre-existing
// user/tenant token path while the active profile is an Agent Employee. That
// path has no way to carry or check the verified principal such a credential
// must be matched against, so producing anything there would mean handing back
// a credential nobody validated. Reaching it means a call site was not moved to
// the access-resolution entry point.
func AgentLegacyTokenError() error {
	return errs.NewInternalError(errs.SubtypeSDKError,
		"an Agent Employee profile reached the user or tenant token path, which cannot verify its principal")
}

// checkResolvedAccess is the post-resolution gate. It re-checks the facts that
// the resolver (local or external) claims, so a provider cannot widen its own
// authority by returning a credential labelled as something it is not.
func checkResolvedAccess(acct *Account, spec AccessSpec, access *ResolvedAccess, now time.Time) error {
	if access == nil {
		return errs.NewInternalError(errs.SubtypeSDKError,
			"credential resolution returned no access")
	}
	if access.Value == "" {
		return errs.NewAuthenticationError(errs.SubtypeTokenMissing,
			"credential resolution returned an empty token for %s", spec.Identity)
	}

	wantIdentity, known := identityForKind(access.Kind)
	if !known {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"credential resolution returned an unsupported credential kind %q", access.Kind)
	}
	if wantIdentity != spec.Identity {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"credential kind %q cannot serve %s identity", access.Kind, spec.Identity)
	}
	// An Agent Employee profile has exactly one credential, so classification and
	// resolution must agree in both directions: it may not produce anything else,
	// and nothing else may produce it.
	//
	// The uat/tat classifications are deliberately not held to the same rule.
	// They describe which credential serves the *user* lane, not the whole
	// account: a profile classified uat can still make bot calls, which legally
	// resolve to a tenant token. Requiring equality there would reject every bot
	// call on such a profile.
	if acct != nil &&
		(acct.CredentialKind == core.CredentialAAT || access.Kind == core.CredentialAAT) &&
		acct.CredentialKind != access.Kind {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"profile is classified as %q but resolution produced %q",
			acct.CredentialKind, access.Kind)
	}

	if access.Kind != core.CredentialAAT {
		return nil
	}
	// No declared principal means the provider owns this credential's identity
	// and lifecycle; there is nothing local to hold it to, so the remaining
	// checks have no reference to compare against.
	if acct == nil || acct.PrincipalOpenID == "" {
		return nil
	}
	if access.OpenID == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Agent Employee access has no principal open ID")
	}
	if access.OpenID != acct.PrincipalOpenID {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"the Agent Employee principal changed").
			WithHint("Rerun `lark-cli config init` to verify and bind the current Agent Employee.")
	}
	if access.ExpiresAt.IsZero() {
		// Reporting this as "expired" would send an integrator hunting a clock or
		// TTL bug, when the real problem is that no expiry was reported at all.
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Agent Employee access reports no expiry, which is required because it cannot be refreshed")
	}
	if !access.ExpiresAt.After(now) {
		return errs.NewAuthenticationError(errs.SubtypeTokenExpired,
			"the resolved Agent Employee token is already expired")
	}
	return nil
}
