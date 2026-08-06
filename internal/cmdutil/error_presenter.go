// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
)

// ErrorPresentationOptions supplies invocation-specific facts to PresentError.
// DeclaredScopes is lazy because only user-authorization recovery with no
// server-reported scope facts needs command metadata resolution.
type ErrorPresentationOptions struct {
	Projector      *recovery.Projector
	Identity       core.Identity
	DeclaredScopes func() []string
}

// PresentError clones and projects a typed producer error before command-facing
// fields are copied into either a root error envelope or a result payload. The
// producer is never mutated, and all machine-readable fields are preserved.
func (f *Factory) PresentError(err error, options ErrorPresentationOptions) error {
	if err == nil || errs.IsRaw(err) {
		return err
	}

	projector := options.Projector
	if projector == nil && f != nil {
		projector = f.Recovery
	}
	rendered := projector.Render(err)
	completePermissionRecovery(f, rendered, projector, options.Identity, options.DeclaredScopes)
	applyNeedAuthorizationHint(rendered, projector, options.DeclaredScopes)
	return rendered
}

func completePermissionRecovery(
	f *Factory,
	err error,
	projector *recovery.Projector,
	identity core.Identity,
	declaredScopes func() []string,
) {
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return
	}
	permissionErr, ok := typed.(*errs.PermissionError) //nolint:errorlint // presentation must not descend into the clone's original Cause
	if !ok {
		return
	}
	if permissionErr.Identity != "" {
		identity = core.Identity(permissionErr.Identity)
	} else if identity == "" && f != nil {
		identity = f.ResolvedIdentity
	}
	if identity == "" {
		identity = core.AsUser
	}
	canonical := errclass.PermissionRecovery(
		permissionErr.MissingScopes,
		string(identity),
		permissionErr.Subtype,
		permissionErr.ConsoleURL,
	)
	if permissionErr.Hint != "" &&
		permissionErr.Hint != canonical.String() &&
		permissionErr.Hint != projector.RenderHint(canonical) {
		return
	}

	recoveryScopes := permissionErr.MissingScopes
	if permissionRecoveryUsesDeclaredScopes(permissionErr, identity) && declaredScopes != nil {
		if scopes := declaredScopes(); len(scopes) > 0 {
			recoveryScopes = scopes
		}
	}
	hint := errclass.PermissionRecovery(
		recoveryScopes,
		string(identity),
		permissionErr.Subtype,
		permissionErr.ConsoleURL,
	)
	permissionErr.Hint = projector.RenderHint(hint)
}

func permissionRecoveryUsesDeclaredScopes(permissionErr *errs.PermissionError, identity core.Identity) bool {
	if permissionErr == nil || identity != core.AsUser || len(permissionErr.MissingScopes) > 0 {
		return false
	}
	switch permissionErr.Subtype {
	case errs.SubtypeMissingScope, errs.SubtypeTokenScopeInsufficient, errs.SubtypeUserUnauthorized:
		return true
	default:
		return false
	}
}

func applyNeedAuthorizationHint(err error, projector *recovery.Projector, declaredScopes func() []string) {
	if err == nil || declaredScopes == nil || !internalauth.IsNeedUserAuthorizationError(err) {
		return
	}
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return
	}
	authErr, ok := typed.(*errs.AuthenticationError) //nolint:errorlint // enrich only the presented clone, never a nested producer Cause
	if !ok {
		return
	}
	scopes := declaredScopes()
	if len(scopes) == 0 {
		return
	}
	scopedRecovery := projector.RenderHint(recovery.UserAuthorization(scopes...))
	genericRecovery := projector.RenderHint(recovery.UserAuthorization())
	if authErr.Hint == "" || authErr.Hint == genericRecovery {
		authErr.Hint = scopedRecovery
		return
	}
	authErr.Hint += "\n" + scopedRecovery
}
