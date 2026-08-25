// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/registry"
)

// presentRootError uses the same build-local presenter as shortcut result
// sinks, adding only the root command's lazy declared-scope resolver.
func presentRootError(f *cmdutil.Factory, err error, projector *recovery.Projector) error {
	identity := core.Identity("")
	if f != nil {
		identity = f.ResolvedIdentity
	}
	return f.PresentError(err, cmdutil.ErrorPresentationOptions{
		Projector: projector,
		Identity:  identity,
		DeclaredScopes: func() []string {
			return resolveDeclaredScopesForCurrentCommand(f)
		},
	})
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
	return cmdmeta.DeclaredScopes(cmd, identity)
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
