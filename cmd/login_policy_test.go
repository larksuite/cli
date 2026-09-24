// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
)

func TestBuildLoginCommandEligibility(t *testing.T) {
	tmpHome(t)
	t.Cleanup(cmdpolicy.ResetActiveForTesting)
	plugin := platform.NewPlugin("login-policy", "1").Restrict(&platform.Rule{
		Deny: []string{"base/**", "contact/+search-bot", "wiki/spaces/get"}, AllowUnannotated: true,
	}).MustBuild()
	result, err := buildForArgs(context.Background(), cmdutil.InvocationContext{}, []string{"auth", "login"},
		WithIO(strings.NewReader(""), io.Discard, io.Discard), WithoutStrictMode(), ConcealRestrictedCommands(),
		func(cfg *buildConfig) {
			cfg.pluginProvider = func() []platform.Plugin { return []platform.Plugin{plugin} }
		})
	if err != nil {
		t.Fatal(err)
	}
	// A later build must not change the earlier build's login selection.
	ordinary, _, _ := buildInternal(context.Background(), cmdutil.InvocationContext{}, WithoutPlugins(), WithoutStrictMode())
	if ordinary.LoginCommandAllowed != nil {
		t.Fatal("unrestricted build must retain remote-first scope discovery")
	}
	for path, want := range map[string]bool{
		"base/+dashboard-create": false, "contact/+search-bot": false,
		"wiki/spaces/get": false, "wiki/spaces/list": true,
	} {
		if got := result.runtime.LoginCommandAllowed(strings.Split(path, "/")); got != want {
			t.Errorf("%s allowed = %v, want %v", path, got, want)
		}
	}
}
