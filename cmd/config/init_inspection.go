// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/keychain"
)

// initCredentialInspectionTimeout bounds the two calls credential inspection
// makes. It is deliberately larger than probeTimeout — a wrong verdict here
// misclassifies the profile, so a slow-but-working network is worth waiting for
// — and deliberately far below the old 30s, because the whole step is
// best-effort and every second of it is spent in front of a user who has just
// typed an app secret. A variable so tests can exercise the timeout path
// without waiting it out.
var initCredentialInspectionTimeout = 10 * time.Second

type keychainValueSnapshot struct {
	account string
	value   string
	exists  bool
}

// snapshotKeychainValue records the current entry so a failed init can put it
// back. A read failure is deliberately not an error: storage that cannot be
// read is exactly the state `lark-cli config init` is documented to repair —
// with the master key gone, Get fails while Set regenerates the key and
// succeeds — so refusing to start would close the only recovery door the CLI
// offers. An unreadable entry is recorded as absent, which degrades rollback
// from "restore the old value" to "remove what this run wrote"; the old value
// was already unreadable, so nothing recoverable is lost.
func snapshotKeychainValue(kc keychain.KeychainAccess, account string) keychainValueSnapshot {
	value, err := kc.Get(keychain.LarkCliService, account)
	if err != nil {
		return keychainValueSnapshot{account: account}
	}
	return keychainValueSnapshot{account: account, value: value, exists: value != ""}
}

func restoreKeychainValue(kc keychain.KeychainAccess, snapshot keychainValueSnapshot) error {
	if snapshot.exists {
		if err := kc.Set(keychain.LarkCliService, snapshot.account, snapshot.value); err != nil {
			return errs.NewInternalError(errs.SubtypeStorage,
				"failed to restore credential storage entry %q", snapshot.account).WithCause(err)
		}
		return nil
	}
	if err := kc.Remove(keychain.LarkCliService, snapshot.account); err != nil && !errors.Is(err, keychain.ErrNotFound) {
		return errs.NewInternalError(errs.SubtypeStorage,
			"failed to restore credential storage entry %q", snapshot.account).WithCause(err)
	}
	return nil
}

// persistInspectedInit owns the credential-inspection config-init transaction. It does
// not write any local state until client_credentials mint and Introspection
// have both succeeded. Agent Employee facts and the verified AAT are then
// committed together with the app secret and profile classification.
//
// The two network calls are judged differently. The token endpoint answers
// "are these credentials valid", and a deterministic no there blocks: saving
// them would only defer the same failure to the first real command.
// Introspection answers "is this app an Agent Employee", and any answer other
// than yes -- transport failure, 5xx, inactive, another client id, an
// unparseable body -- means that question went unanswered, not that the
// credentials are bad. Those fall open to an unclassified profile, the state
// config init produced before inspection existed.
func persistInspectedInit(
	ctx context.Context,
	f *cmdutil.Factory,
	profileName string,
	existing *core.MultiAppConfig,
	appID, appSecret string,
	brand core.LarkBrand,
	lang string,
) (retErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := ctx
	httpClient, err := f.HttpClient()
	if err != nil {
		// No client means inspection never ran, which says nothing about the
		// credentials. Before inspection existed this failure only cost the
		// probe; it must not now cost the user their configuration.
		return persistUninspectedInit(parent, f, profileName, existing, appID, appSecret, brand, lang, err)
	}
	ctx, cancel := context.WithTimeout(ctx, initCredentialInspectionTimeout)
	defer cancel()

	minted, err := credential.FetchClientCredentialsToken(ctx, httpClient, brand, appID, appSecret)
	if err != nil {
		if tokenEndpointRejects(err) {
			return err
		}
		if interrupted := initInterruptedError(parent); interrupted != nil {
			return interrupted
		}
		return persistUninspectedInit(parent, f, profileName, existing, appID, appSecret, brand, lang, err)
	}
	if minted.ExpiresAt.IsZero() || !minted.ExpiresAt.After(minted.GrantedAt) {
		// The mint succeeded but cannot be cached as an Agent token, so the
		// question of whether this is one is left unanswered rather than
		// answered with a record that is already spent.
		err = errs.NewInternalError(errs.SubtypeInvalidResponse,
			"client_credentials response has no valid expiry")
		return persistUninspectedInit(parent, f, profileName, existing, appID, appSecret, brand, lang, err)
	}
	introspection, err := credential.IntrospectAccessToken(ctx, httpClient, brand, appID, appSecret, minted.Token)
	if err != nil {
		if interrupted := initInterruptedError(parent); interrupted != nil {
			return interrupted
		}
		// The fail-open path gets the caller's context, not the inspection
		// budget: on a slow network that budget is exactly what just ran out,
		// and the probe would fail before it started.
		return persistUninspectedInit(parent, f, profileName, existing, appID, appSecret, brand, lang, err)
	}

	secretSnapshot := snapshotKeychainValue(f.Keychain, core.AppSecretAccountKey(appID))
	aatSnapshot := snapshotKeychainValue(f.Keychain, auth.AATAccountKey(appID))

	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackErr := errors.Join(
			restoreKeychainValue(f.Keychain, aatSnapshot),
			restoreKeychainValue(f.Keychain, secretSnapshot),
		)
		if rollbackErr == nil {
			return
		}
		retErr = errs.NewInternalError(errs.SubtypeStorage,
			"credential initialization failed and local credential rollback was incomplete").
			WithCause(errors.Join(retErr, rollbackErr)).
			WithHint("Local credentials may be inconsistent. Fix access to config and keychain storage, then rerun `lark-cli config init`.")
	}()

	storedSecret, err := core.ForStorage(appID, core.PlainSecret(appSecret), f.Keychain)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"failed to store app secret").WithCause(err)
	}

	principalOpenID := ""
	agentScope := ""
	if introspection.IsAgentEmployee() {
		principalOpenID = introspection.OpenID
		agentScope = minted.Scope
		if err := auth.SetStoredAAT(f.Keychain, &auth.StoredAAToken{
			AppID:       appID,
			UserOpenID:  principalOpenID,
			AccessToken: minted.Token,
			ExpiresAt:   minted.ExpiresAt.UnixMilli(),
			GrantedAt:   minted.GrantedAt.UnixMilli(),
			Scope:       minted.Scope,
		}); err != nil {
			return err
		}
	}

	next, err := buildInspectedInitConfig(
		existing, profileName, appID, storedSecret, brand, lang, principalOpenID, agentScope)
	if err != nil {
		return err
	}
	if err := core.SaveMultiAppConfig(next); err != nil {
		return wrapSaveConfigError(err)
	}
	committed = true

	cleanupReplacedInitProfiles(existing, f, profileName, appID)
	if !introspection.IsAgentEmployee() {
		_ = auth.RemoveStoredAAT(f.Keychain, appID)
		// Report the setup exactly as config init did before inspection
		// existed. The token minted above is a tenant token for this app, so
		// the probe costs no extra round trip. An Agent Employee is skipped:
		// its minted token is an Agent token, and the probe endpoint takes a
		// tenant one. The budget is fresh rather than whatever inspection left
		// over, so a slow verification does not silently drop the report.
		probeCtx, probeCancel := context.WithTimeout(parent, probeTimeout)
		postAppProbe(probeCtx, httpClient, brand, minted.Token)
		probeCancel()
	}
	return nil
}

func buildInspectedInitConfig(
	existing *core.MultiAppConfig,
	profileName, appID string,
	secret core.SecretInput,
	brand core.LarkBrand,
	lang, principalOpenID, agentScope string,
) (*core.MultiAppConfig, error) {
	multi := cloneMultiAppConfig(existing)
	credentialKind := core.CredentialKind("")
	if principalOpenID != "" {
		credentialKind = core.CredentialAAT
	}
	if profileName == "" {
		var prior i18n.Lang
		if existing != nil {
			if app := existing.CurrentAppConfig(""); app != nil {
				prior = app.Lang
			}
		}
		app := core.AppConfig{
			AppId:           appID,
			AppSecret:       secret,
			Brand:           brand,
			Lang:            preferredLang(i18n.Lang(lang), prior),
			Users:           []core.AppUser{},
			PrincipalOpenID: principalOpenID,
			CredentialKind:  credentialKind,
			AgentScope:      agentScope,
		}
		return &core.MultiAppConfig{Apps: []core.AppConfig{app}}, nil
	}

	if idx := findProfileIndexByName(multi, profileName); idx >= 0 {
		app := &multi.Apps[idx]
		if app.AppId != appID {
			app.Users = []core.AppUser{}
		}
		app.AppId = appID
		app.AppSecret = secret
		app.Brand = brand
		app.Lang = preferredLang(i18n.Lang(lang), app.Lang)
		app.PrincipalOpenID = principalOpenID
		app.CredentialKind = credentialKind
		app.AgentScope = agentScope
		return multi, nil
	}
	if findAppIndexByAppID(multi, profileName) >= 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"profile name %q conflicts with existing appId", profileName).
			WithParam("--name")
	}
	app := core.AppConfig{
		Name:            profileName,
		AppId:           appID,
		AppSecret:       secret,
		Brand:           brand,
		Lang:            i18n.Lang(lang),
		Users:           []core.AppUser{},
		PrincipalOpenID: principalOpenID,
		CredentialKind:  credentialKind,
		AgentScope:      agentScope,
	}
	multi.Apps = append(multi.Apps, app)
	return multi, nil
}

func cloneMultiAppConfig(existing *core.MultiAppConfig) *core.MultiAppConfig {
	if existing == nil {
		return &core.MultiAppConfig{}
	}
	copyOfConfig := *existing
	copyOfConfig.Apps = append([]core.AppConfig(nil), existing.Apps...)
	for i := range copyOfConfig.Apps {
		copyOfConfig.Apps[i].Users = append([]core.AppUser(nil), existing.Apps[i].Users...)
	}
	return &copyOfConfig
}

func cleanupReplacedInitProfiles(
	existing *core.MultiAppConfig,
	f *cmdutil.Factory,
	profileName, keepAppID string,
) {
	if existing == nil {
		return
	}
	if profileName == "" {
		cleanupOldConfig(existing, f, keepAppID)
		for _, app := range existing.Apps {
			if app.AppId != keepAppID {
				_ = auth.RemoveStoredAAT(f.Keychain, app.AppId)
			}
		}
		return
	}
	idx := findProfileIndexByName(existing, profileName)
	if idx < 0 || existing.Apps[idx].AppId == keepAppID {
		return
	}
	old := existing.Apps[idx]
	core.RemoveSecretStore(old.AppSecret, f.Keychain)
	for _, user := range old.Users {
		_ = auth.RemoveStoredToken(old.AppId, user.UserOpenId)
	}
	_ = auth.RemoveStoredAAT(f.Keychain, old.AppId)
}

// tokenEndpointRejects decides whether a failed client_credentials mint may
// stop config init. Only a deterministic verdict about the credentials
// themselves blocks: FetchClientCredentialsToken types a rejection such as
// invalid_client (config) and leaves transport failures, upstream 5xx and
// undecodable bodies untyped; rate limiting is typed but as a retryable API
// error, which is not a verdict either. A standard app must not be locked out
// of the CLI because the accounts endpoint was unreachable at the moment it was
// configured.
func tokenEndpointRejects(err error) bool {
	problem, typed := errs.ProblemOf(err)
	if !typed {
		return false
	}
	switch problem.Category {
	case errs.CategoryConfig, errs.CategoryAuthentication, errs.CategoryValidation:
		return true
	}
	return false
}

// initInterruptedError reports a cancelled parent context as the refusal
// config init gives when the user interrupts it. Interrupting the command is an
// instruction not to change anything; falling open would write the very
// configuration the user just cancelled, and report success doing it.
func initInterruptedError(parent context.Context) error {
	if parent.Err() == nil {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"config init was interrupted before the credentials could be verified; nothing was changed").
		WithCause(parent.Err())
}

// persistUninspectedInit is the fail-open path: inspection could not run to a
// verdict, so the profile is saved exactly as config init saved it before
// inspection existed — unclassified, no AAT cache, followed by the best-effort
// probe. The one exception is a profile already bound to an Agent Employee:
// rewriting it as a standard app on the strength of credentials nobody could
// verify would silently drop the binding, so that case is refused.
func persistUninspectedInit(
	ctx context.Context,
	f *cmdutil.Factory,
	profileName string,
	existing *core.MultiAppConfig,
	appID, appSecret string,
	brand core.LarkBrand,
	lang string,
	inspectErr error,
) error {
	if target := agentProfileTargetedByInit(existing, profileName, appID); target != "" {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"cannot verify the Agent Employee binding for profile %q: %v", target, inspectErr).
			WithCause(inspectErr).
			WithHint("The profile was left unchanged. Retry once the accounts endpoint is reachable, or remove the profile first if it should no longer be an Agent Employee.")
	}
	if f.IOStreams != nil && f.IOStreams.ErrOut != nil {
		fmt.Fprintf(f.IOStreams.ErrOut,
			"warning: could not determine whether app %s is an Agent Employee (%v); saved as a standard app. "+
				"If it is an Agent Employee, rerun `lark-cli config init` once the accounts endpoint is reachable.\n",
			appID, inspectErr)
	}
	storedSecret, err := core.ForStorage(appID, core.PlainSecret(appSecret), f.Keychain)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
	}
	if err := saveInitConfig(profileName, existing, f, appID, storedSecret, brand, lang); err != nil {
		return wrapSaveConfigError(err)
	}
	return runProbe(ctx, f, appID, appSecret, brand)
}

// agentProfileTargetedByInit names the existing Agent Employee profile that
// this init would overwrite, or "" when none is affected. A named init targets
// that profile; an unnamed init replaces the whole config, so any Agent
// Employee entry for the same app counts.
func agentProfileTargetedByInit(existing *core.MultiAppConfig, profileName, appID string) string {
	if existing == nil {
		return ""
	}
	if profileName != "" {
		if idx := findProfileIndexByName(existing, profileName); idx >= 0 &&
			existing.Apps[idx].CredentialKind == core.CredentialAAT {
			return existing.Apps[idx].ProfileName()
		}
		return ""
	}
	for _, app := range existing.Apps {
		if app.AppId == appID && app.CredentialKind == core.CredentialAAT {
			return app.ProfileName()
		}
	}
	return ""
}
