// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	shortcutcommon "github.com/larksuite/cli/shortcuts/common"
)

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
	var authErr *errs.AuthenticationError
	if !errors.As(err, &authErr) {
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

// enrichMissingScopeError appends a "current command requires scope(s): X"
// hint to a legacy *output.ExitError when the underlying error carries the
// need_user_authorization marker AND the current command declares scopes
// locally.
//
// Deprecated: enrichment for the legacy envelope; the typed path is
// applyNeedAuthorizationHint above.
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
		if sc.GetService() != service || sc.GetCommand() != cmd.Name() || !shortcutSupportsIdentity(sc, identity) {
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
	return registry.DeclaredScopesForMethod(methodMap, identity)
}

// shortcutSupportsIdentity reports whether a shortcut supports the requested
// identity, applying the default user-only behavior when AuthTypes is empty.
func shortcutSupportsIdentity(sc shortcutcommon.ShortcutDescriptor, identity string) bool {
	authTypes := sc.GetAuthTypes()
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
