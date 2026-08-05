// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	shortcutcommon "github.com/larksuite/cli/shortcuts/common"
)

// rootErrorPresenter owns the final command-facing error transformation for
// one Cobra tree. Producers report typed facts and optional semantic recovery;
// this boundary clones, completes, and projects them without exposing the
// build-local surface plan to business packages.
type rootErrorPresenter struct {
	f         *cmdutil.Factory
	projector *recovery.Projector
}

func newRootErrorPresenter(f *cmdutil.Factory, projector *recovery.Projector) *rootErrorPresenter {
	return &rootErrorPresenter{f: f, projector: projector}
}

func (p *rootErrorPresenter) Present(err error) error {
	if err == nil || errs.IsRaw(err) {
		return err
	}
	rendered := p.projector.Render(err)
	p.completePermissionRecovery(rendered)
	applyNeedAuthorizationHint(p.f, rendered)
	return rendered
}

// completePermissionRecovery supplies the canonical recovery for direct
// PermissionError producers. API classification paths that already carry an
// owned structured annotation keep their rendered Hint unchanged.
func (p *rootErrorPresenter) completePermissionRecovery(err error) {
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return
	}
	permissionErr, ok := typed.(*errs.PermissionError) //nolint:errorlint // presentation must not descend into the clone's original Cause
	if !ok || permissionErr.Hint != "" {
		return
	}
	identity := permissionErr.Identity
	if identity == "" && p.f != nil {
		identity = string(p.f.ResolvedIdentity)
	}
	if identity == "" {
		identity = string(core.AsUser)
	}
	hint := errclass.PermissionRecovery(
		permissionErr.MissingScopes,
		identity,
		permissionErr.Subtype,
		permissionErr.ConsoleURL,
	)
	permissionErr.Hint = p.projector.RenderHint(hint)
}

// applyNeedAuthorizationHint augments a typed *errs.AuthenticationError with a
// "current command requires scope(s): X, Y" hint when the underlying error is
// a need_user_authorization signal AND the current command declares scopes
// locally (via shortcut registration or service-method metadata). Existing
// Hint text is preserved; scopes are appended on a new line.
func applyNeedAuthorizationHint(f *cmdutil.Factory, err error) {
	if err == nil || f == nil {
		return
	}
	if !internalauth.IsNeedUserAuthorizationError(err) {
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
	scopes := resolveDeclaredScopesForCurrentCommand(f)
	if len(scopes) == 0 {
		return
	}
	scopeHint := fmt.Sprintf("current command requires scope(s): %s", strings.Join(scopes, ", "))
	if authErr.Hint == "" {
		authErr.Hint = scopeHint
		return
	}
	authErr.Hint += "\n" + scopeHint
}

// resolveDeclaredScopesForCurrentCommand returns the scopes declared by the
// current command for the resolved identity, checking shortcuts first and then
// service methods from local registry metadata.
func resolveDeclaredScopesForCurrentCommand(f *cmdutil.Factory) []string {
	if f == nil || f.CurrentCommand == nil {
		return nil
	}

	identity := string(f.ResolvedIdentity)
	if identity == "" {
		identity = string(core.AsUser)
	}
	if identity != string(core.AsUser) && identity != string(core.AsBot) {
		return nil
	}

	if scopes := resolveDeclaredShortcutScopes(f.CurrentCommand, identity); len(scopes) > 0 {
		return scopes
	}
	return resolveDeclaredServiceMethodScopes(f.CurrentCommand, identity)
}

// resolveDeclaredShortcutScopes returns the scopes declared by a mounted
// shortcut command for the given identity.
func resolveDeclaredShortcutScopes(cmd *cobra.Command, identity string) []string {
	if cmd == nil || cmd.Parent() == nil || !strings.HasPrefix(cmd.Name(), "+") {
		return nil
	}

	service := cmd.Parent().Name()
	for _, sc := range shortcuts.AllShortcuts() {
		if sc.Service != service || sc.Command != cmd.Name() || !shortcutSupportsIdentity(sc, identity) {
			continue
		}
		scopes := sc.DeclaredScopesForIdentity(identity)
		if len(scopes) == 0 {
			return nil
		}
		return append([]string(nil), scopes...)
	}
	return nil
}

// resolveDeclaredServiceMethodScopes returns the scopes declared by a
// service/resource/method command. It reconstructs the catalog path from the
// command ancestry and resolves it through the same navigation Module the
// command tree is built from (apicatalog), so it stays correct for nested
// resources instead of hard-coding a root->service->resource->method depth.
// Non-method commands (services, resources, shortcuts) resolve to a non-method
// target and yield no scopes.
func resolveDeclaredServiceMethodScopes(cmd *cobra.Command, identity string) []string {
	if cmd == nil || strings.HasPrefix(cmd.Name(), "+") {
		return nil
	}
	path := commandCatalogPath(cmd)
	if len(path) == 0 {
		return nil
	}
	target, err := registry.RuntimeCatalog().Resolve(path)
	if err != nil || target.Kind != apicatalog.TargetMethod {
		return nil
	}
	return registry.DeclaredScopesForMethod(target.Method.Method, identity)
}

// commandCatalogPath reconstructs the catalog path [service, resource..., method]
// from a command's ancestry, excluding the root command. It is the inverse of
// the service command tree's construction, so any depth (flat or nested)
// round-trips through apicatalog.Resolve.
func commandCatalogPath(cmd *cobra.Command) []string {
	var path []string
	for c := cmd; c != nil && c.Parent() != nil; c = c.Parent() {
		path = append([]string{c.Name()}, path...)
	}
	return path
}

// shortcutSupportsIdentity reports whether a shortcut supports the requested
// identity, applying the default user-only behavior when AuthTypes is empty.
func shortcutSupportsIdentity(sc shortcutcommon.Shortcut, identity string) bool {
	authTypes := sc.AuthTypes
	if len(authTypes) == 0 {
		authTypes = []string{string(core.AsUser)}
	}
	for _, authType := range authTypes {
		if authType == identity {
			return true
		}
	}
	return false
}
