// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strings"

	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	shortcutcommon "github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// enrichMissingScopeError preserves the original need_user_authorization
// message and appends a scope hint when the current command declares the
// required scopes locally.
func enrichMissingScopeError(f *cmdutil.Factory, exitErr *output.ExitError) {
	if exitErr == nil || exitErr.Detail == nil {
		return
	}
	if !internalauth.IsNeedUserAuthorizationError(exitErr) {
		return
	}

	scopes := resolveDeclaredScopesForCurrentCommand(f)
	if len(scopes) == 0 {
		return
	}

	scopeHint := fmt.Sprintf("current command requires scope(s): %s", strings.Join(scopes, ", "))
	if exitErr.Detail.Hint == "" {
		exitErr.Detail.Hint = scopeHint
		return
	}
	exitErr.Detail.Hint += "\n" + scopeHint
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
// service/resource/method command from the embedded from_meta registry.
func resolveDeclaredServiceMethodScopes(cmd *cobra.Command, identity string) []string {
	// Service-method scope lookup only applies to commands mounted as
	// root -> service -> resource -> method. Non-resource/method commands
	// intentionally return no scopes here so auth-hint enrichment does not
	// change runtime semantics for other command shapes.
	if cmd == nil || cmd.Parent() == nil || cmd.Parent().Parent() == nil || cmd.Parent().Parent().Parent() == nil {
		return nil
	}
	if strings.HasPrefix(cmd.Name(), "+") {
		return nil
	}

	service := cmd.Parent().Parent().Name()
	resource := cmd.Parent().Name()
	method := cmd.Name()

	spec := registry.LoadFromMeta(service)
	if spec == nil {
		return nil
	}
	resources, _ := spec["resources"].(map[string]interface{})
	resMap, _ := resources[resource].(map[string]interface{})
	if resMap == nil {
		return nil
	}
	methods, _ := resMap["methods"].(map[string]interface{})
	methodMap, _ := methods[method].(map[string]interface{})
	if methodMap == nil {
		return nil
	}
	return declaredScopesForMethod(methodMap, identity)
}

// declaredScopesForMethod returns all requiredScopes when present; otherwise it
// resolves the single recommended scope from the method's scopes list.
func declaredScopesForMethod(method map[string]interface{}, identity string) []string {
	if requiredRaw, ok := method["requiredScopes"].([]interface{}); ok && len(requiredRaw) > 0 {
		return interfaceStrings(requiredRaw)
	}

	rawScopes, _ := method["scopes"].([]interface{})
	if len(rawScopes) == 0 {
		return nil
	}
	recommended := registry.SelectRecommendedScope(rawScopes, identity)
	if recommended == "" {
		for _, raw := range rawScopes {
			if scope, ok := raw.(string); ok && scope != "" {
				recommended = scope
				break
			}
		}
	}
	if recommended == "" {
		return nil
	}
	return []string{recommended}
}

// interfaceStrings converts a []interface{} containing strings into a compact
// []string, skipping empty or non-string values.
func interfaceStrings(values []interface{}) []string {
	scopes := make([]string, 0, len(values))
	for _, value := range values {
		scope, ok := value.(string)
		if !ok || scope == "" {
			continue
		}
		scopes = append(scopes, scope)
	}
	return scopes
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
