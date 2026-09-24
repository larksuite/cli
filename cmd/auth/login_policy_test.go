// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestAuthLoginPolicyScopes(t *testing.T) {
	oldFetch := fetchRemoteScopes
	t.Cleanup(func() { fetchRemoteScopes = oldFetch })
	fetchRemoteScopes = func(core.LarkBrand) (map[string][]string, bool) {
		t.Fatal("policy login must not fetch remote scopes")
		return nil, false
	}
	resolver := newDomainResolver(apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{{
		Name: "contact", Resources: map[string]meta.Resource{"users": {Methods: map[string]meta.Method{
			"get":     {Scopes: []string{"contact:read"}},
			"removed": {Scopes: []string{"contact:removed"}},
		}}},
	}}), []common.Shortcut{
		{Service: "contact", Command: "+search-bot", Scopes: []string{"search:bot", "contact:shared"}},
		{Service: "contact", Command: "+removed", Scopes: []string{"contact:removed", "contact:shared"}},
	})
	// Exercise policy selection independently of external-command detection.
	resolver.hasExternal = false
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, http := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_test", AppSecret: "test", Brand: core.BrandFeishu})
	f.LoginCommandAllowed = func(path []string) bool {
		return !strings.Contains(strings.Join(path, "/"), "removed")
	}
	stub := &httpmock.Stub{Method: "POST", URL: larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{"device_code": "fixture-device", "verification_uri": "https://example.com/verify"}}
	http.Register(stub)
	if err := authLoginRun(&LoginOptions{Factory: f, Ctx: context.Background(), NoWait: true}, resolver); err != nil {
		t.Fatal(err)
	}
	form, err := url.ParseQuery(string(stub.CapturedBody))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(form.Get("scope"))
	slices.Sort(got)
	want := []string{"contact:read", "contact:shared", "offline_access", "search:bot"}
	if !slices.Equal(got, want) {
		t.Fatalf("wire scopes = %v, want %v", got, want)
	}
	http.Verify(t)
}
