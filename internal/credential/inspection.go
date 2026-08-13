// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"strings"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
)

// ScopeState describes whether a credential source can authoritatively report
// the scopes granted to a token. It deliberately distinguishes an omitted
// answer from a source that does not support scope inspection.
type ScopeState string

const (
	ScopeKnown       ScopeState = "known"
	ScopeUnknown     ScopeState = "unknown"
	ScopeUnsupported ScopeState = "unsupported"
)

// TokenInspectionStatus is the local, non-secret state of a token or identity.
type TokenInspectionStatus string

const (
	TokenInspectionReady         TokenInspectionStatus = "ready"
	TokenInspectionNeedsRefresh  TokenInspectionStatus = "needs_refresh"
	TokenInspectionExpired       TokenInspectionStatus = "expired"
	TokenInspectionMissing       TokenInspectionStatus = "missing"
	TokenInspectionNotLoggedIn   TokenInspectionStatus = "not_logged_in"
	TokenInspectionNotSupported  TokenInspectionStatus = "not_supported"
	TokenInspectionAvailableLive TokenInspectionStatus = "available_on_demand"
)

// SourceInspection is a sanitized description of the selected credential
// source. It never contains an app secret or any resolved credential value.
type SourceInspection struct {
	Name                 string
	Managed              bool
	AppID                string
	Brand                core.LarkBrand
	DefaultAs            core.Identity
	ProfileName          string
	UserOpenID           string
	UserName             string
	SupportedIdentities  uint8
	ProvidesOnDemandAuth bool
	CanInspectScopes     bool
}

// TokenInspectionRequest controls a non-secret token inspection.
type TokenInspectionRequest struct {
	TokenSpec
	IncludeScopes bool
}

// TokenInspection contains only diagnostic metadata. In particular, it has no
// field capable of carrying the resolved credential value.
type TokenInspection struct {
	Source                 SourceInspection
	Status                 TokenInspectionStatus
	Present                bool
	ScopeState             ScopeState
	Scopes                 string
	ExpiresAtMillis        int64
	RefreshExpiresAtMillis int64
	GrantedAtMillis        int64
}

// InspectSource returns a sanitized view of the selected credential source.
// Extension detection remains encapsulated here so commands do not need to
// know whether credentials came from env, a helper, or the built-in keychain.
func (p *CredentialProvider) InspectSource(ctx context.Context) (*SourceInspection, error) {
	if p == nil {
		return &SourceInspection{Name: "default"}, nil
	}
	acct, err := p.ResolveAccount(ctx)
	info := &SourceInspection{Name: "default"}
	if p.selectedProvider != nil {
		info.Managed = true
		info.Name = p.selectedProvider.Name()
		if info.Name == "" {
			info.Name = "external"
		}
	}
	if err != nil {
		// Source inspection must not replace the established error path for the
		// built-in config/keychain source. Commands still load their normal
		// config after this call and report the same command-specific error as
		// before the inspection boundary existed. A selected managed provider,
		// however, owns credential resolution, so its failure remains fail
		// closed and must be surfaced here.
		if info.Managed {
			return info, err
		}
		return info, nil
	}
	if acct == nil {
		return info, nil
	}
	fillSourceInspection(info, acct, p.selectedCaps)
	return info, nil
}

// InspectToken reports token availability and metadata without returning the
// credential value. Scope resolution is opt-in because managed sources may
// need to perform work to obtain authoritative scope metadata.
func (p *CredentialProvider) InspectToken(ctx context.Context, req TokenInspectionRequest) (*TokenInspection, error) {
	acct, err := p.ResolveAccount(ctx)
	if err != nil {
		return nil, err
	}
	source, err := p.selectedCredentialSource(ctx)
	if err != nil {
		return nil, err
	}

	info := SourceInspection{Name: "default"}
	if source != nil {
		info.Name = source.Name()
	}
	if p.selectedProvider != nil {
		info.Managed = true
		if info.Name == "" {
			info.Name = "external"
		}
	}
	if acct != nil {
		fillSourceInspection(&info, acct, p.selectedCaps)
	}

	result := &TokenInspection{
		Source:     info,
		Status:     TokenInspectionMissing,
		ScopeState: ScopeUnknown,
	}
	if acct == nil || source == nil {
		return result, nil
	}

	if info.Managed {
		return inspectManagedToken(ctx, source, acct, req, result)
	}
	return inspectDefaultToken(acct, req.TokenSpec, result), nil
}

func fillSourceInspection(info *SourceInspection, acct *Account, capabilities ProviderCapabilities) {
	info.AppID = acct.AppID
	info.Brand = acct.Brand
	info.DefaultAs = acct.DefaultAs
	info.ProfileName = acct.ProfileName
	info.UserOpenID = acct.UserOpenId
	info.UserName = acct.UserName
	info.SupportedIdentities = acct.SupportedIdentities
	info.ProvidesOnDemandAuth = capabilities.ProvidesOnDemandAuth
	info.CanInspectScopes = capabilities.CanInspectScopes
}

func inspectDefaultToken(acct *Account, spec TokenSpec, result *TokenInspection) *TokenInspection {
	switch spec.Type {
	case TokenTypeTAT:
		ids := extcred.IdentitySupport(acct.SupportedIdentities)
		if ids.UserOnly() {
			result.Status = TokenInspectionNotSupported
			result.ScopeState = ScopeUnsupported
			return result
		}
		if acct.SupportedIdentities == 0 && !HasRealAppSecret(acct.AppSecret) {
			result.Status = TokenInspectionMissing
			result.ScopeState = ScopeUnsupported
			return result
		}
		result.Status = TokenInspectionReady
		result.Present = true
		result.ScopeState = ScopeUnsupported
		return result
	case TokenTypeUAT:
		if acct.UserOpenId == "" {
			result.Status = TokenInspectionNotLoggedIn
			return result
		}
		stored := getStoredToken(acct.AppID, acct.UserOpenId)
		if stored == nil {
			result.Status = TokenInspectionMissing
			return result
		}
		result.Present = true
		result.ScopeState = ScopeKnown
		result.Scopes = stored.Scope
		result.ExpiresAtMillis = stored.ExpiresAt
		result.RefreshExpiresAtMillis = stored.RefreshExpiresAt
		result.GrantedAtMillis = stored.GrantedAt
		switch getStoredTokenStatus(stored) {
		case "valid":
			result.Status = TokenInspectionReady
		case "needs_refresh":
			result.Status = TokenInspectionNeedsRefresh
		default:
			result.Status = TokenInspectionExpired
		}
		return result
	default:
		result.Status = TokenInspectionNotSupported
		result.ScopeState = ScopeUnsupported
		return result
	}
}

func inspectManagedToken(
	ctx context.Context,
	source credentialSource,
	acct *Account,
	req TokenInspectionRequest,
	result *TokenInspection,
) (*TokenInspection, error) {
	if req.Type != TokenTypeUAT && req.Type != TokenTypeTAT {
		result.Status = TokenInspectionNotSupported
		result.ScopeState = ScopeUnsupported
		return result, nil
	}
	ids := extcred.IdentitySupport(acct.SupportedIdentities)
	if (req.Type == TokenTypeUAT && ids.BotOnly()) || (req.Type == TokenTypeTAT && ids.UserOnly()) {
		result.Status = TokenInspectionNotSupported
		result.ScopeState = ScopeUnsupported
		return result, nil
	}

	if result.Source.ProvidesOnDemandAuth {
		result.Status = TokenInspectionAvailableLive
		result.Present = true
	}
	if req.Type == TokenTypeTAT && !result.Source.ProvidesOnDemandAuth {
		result.Status = TokenInspectionReady
		result.Present = true
		result.ScopeState = ScopeUnsupported
	}
	if req.Type == TokenTypeUAT && !result.Source.ProvidesOnDemandAuth && acct.UserOpenId != "" {
		result.Status = TokenInspectionReady
		result.Present = true
	}

	if !req.IncludeScopes {
		return result, nil
	}
	if !result.Source.CanInspectScopes {
		result.ScopeState = ScopeUnsupported
		return result, nil
	}

	token, found, err := source.TryResolveToken(ctx, req.TokenSpec)
	if err != nil {
		return nil, err
	}
	if !found {
		result.Status = TokenInspectionMissing
		result.Present = false
		return result, nil
	}
	result.Status = TokenInspectionReady
	result.Present = true
	if strings.TrimSpace(token.Scopes) == "" {
		result.ScopeState = ScopeUnknown
		return result, nil
	}
	result.ScopeState = ScopeKnown
	result.Scopes = token.Scopes
	return result, nil
}
