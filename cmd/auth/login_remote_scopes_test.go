// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestResolveScopesForDomains_RemoteSendAsUserStillBatchExcluded is the PR B
// regression on the remote path: when the remote scopes.json lists
// im:message.send_as_user, resolveScopesForDomains surfaces it (remote is
// authoritative), but the batch-exclusion filter that authLoginRun applies to
// the resolved set still drops it from the effective wire scopes — matching the
// local path. The remote-fetch seam lives in internal/auth (unreachable from a
// cmd/auth httpmock), so this proves the exclusion at the resolve+filter seam
// rather than end-to-end over the wire.
func TestResolveScopesForDomains_RemoteSendAsUserStillBatchExcluded(t *testing.T) {
	remote := map[string][]string{
		"im": {"im:message", "im:message.send_as_user"},
	}
	resolved := resolveScopesForDomains([]string{"im"}, remote, true, builtinResolver(), core.BrandFeishu)
	if !slices.Contains(resolved, "im:message.send_as_user") {
		t.Fatalf("precondition: remote resolution should surface send_as_user, got %v", resolved)
	}
	effective := filterBatchExcludedScopes(resolved)
	if slices.Contains(effective, "im:message.send_as_user") {
		t.Errorf("send_as_user should be batch-excluded from remote-resolved scopes, got %v", effective)
	}
	if !slices.Contains(effective, "im:message") {
		t.Errorf("non-excluded remote scope im:message should remain, got %v", effective)
	}
}

func TestResolveScopesForDomains_RemoteUsed(t *testing.T) {
	remote := map[string][]string{
		"im":   {"im:message:send", "im:chat:read"},
		"docs": {"docs:doc:read"},
	}
	got := resolveScopesForDomains([]string{"im"}, remote, true, builtinResolver(), core.BrandFeishu)
	want := []string{"im:chat:read", "im:message:send"} // deduped, ascending by sort.Strings
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveScopesForDomains_UnionAcrossDomains(t *testing.T) {
	remote := map[string][]string{
		"im":   {"im:message:send"},
		"docs": {"docs:doc:read"},
	}
	got := resolveScopesForDomains([]string{"im", "docs"}, remote, true, builtinResolver(), core.BrandFeishu)
	want := []string{"docs:doc:read", "im:message:send"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestResolveScopesForDomains_FallbackToLocal(t *testing.T) {
	// remoteOK=false -> falls back to local resolver.scopesFor; im must yield non-empty local scopes
	got := resolveScopesForDomains([]string{"im"}, nil, false, builtinResolver(), core.BrandFeishu)
	if len(got) == 0 {
		t.Fatal("fallback should return non-empty local scopes for im")
	}
}

func TestLegalDomainsFor_RemoteUsed(t *testing.T) {
	// includes "newbiz", a domain unknown to this CLI build, verifying a remote-listed domain is still legal
	remote := map[string][]string{
		"im":     {"im:message:send"},
		"docs":   {"docs:doc:read"},
		"newbiz": {"newbiz:thing:read"},
	}
	set, sorted := legalDomainsFor(remote, true, builtinResolver(), core.BrandFeishu)
	wantSorted := []string{"docs", "im", "newbiz"} // remote keys, ascending by sort.Strings
	if !reflect.DeepEqual(sorted, wantSorted) {
		t.Fatalf("sorted = %v, want %v", sorted, wantSorted)
	}
	if len(set) != len(wantSorted) {
		t.Fatalf("set size = %d, want %d", len(set), len(wantSorted))
	}
	for _, d := range wantSorted {
		if !set[d] {
			t.Errorf("set missing domain %q", d)
		}
	}
	if !set["newbiz"] {
		t.Error("remote-listed domain unknown to this build should still be legal")
	}
}

func TestLegalDomainsFor_FallbackToLocal(t *testing.T) {
	// remoteOK=false -> falls back to local resolver.allKnown/resolver.sorted
	set, sorted := legalDomainsFor(nil, false, builtinResolver(), core.BrandFeishu)
	if len(sorted) == 0 {
		t.Fatal("fallback should return non-empty local domain slice")
	}
	// set and sorted are two views of the same local domain set and must correspond
	if len(set) != len(sorted) {
		t.Fatalf("set size %d != sorted size %d", len(set), len(sorted))
	}
	for _, d := range sorted {
		if !set[d] {
			t.Errorf("set missing local domain %q", d)
		}
	}
	// fallback adopts the local sort order directly, which must equal resolver.sorted
	if want := builtinResolver().sorted(core.BrandFeishu); !reflect.DeepEqual(sorted, want) {
		t.Fatalf("sorted = %v, want resolver.sorted %v", sorted, want)
	}
}

func TestAuthLoginRun_NonTerminal_NoFlags_ProceedsAndSurfacesAgentHint(t *testing.T) {
	// Bare `auth login` no longer rejects in a non-terminal. The interactive
	// picker is gone and bare login now spans the full legal domain set
	// (--recommend ≡ --domain all), so instead of erroring out asking for
	// explicit scopes it proceeds to device authorization and surfaces the
	// agent hint on stderr.
	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		ProfileName: "default",
		AppID:       "cli_test",
		AppSecret:   "secret",
		Brand:       core.BrandFeishu,
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    larkauth.PathDeviceAuthorization,
		Body: map[string]interface{}{
			"device_code":               "device-code",
			"user_code":                 "user-code",
			"verification_uri":          "https://example.com/verify",
			"verification_uri_complete": "https://example.com/verify?code=123",
			"expires_in":                240,
			"interval":                  5,
		},
	})

	// Cancel the context so token polling returns promptly after the device
	// authorization step, which is where the stderr hint is printed.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// TestFactory has IsTerminal=false by default and no scope/domain/recommend
	// flags are set, so this is the bare non-terminal path.
	err := authLoginRun(&LoginOptions{Factory: f, Ctx: ctx}, builtinResolver())
	if err == nil {
		t.Fatal("expected error from cancelled poll after device authorization")
	}
	// Reaching device authorization (verification URL on stderr) proves the
	// old "please specify the scopes" early-reject is gone.
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "https://example.com/verify?code=123") {
		t.Fatalf("expected device authorization to be reached (verification URL on stderr), got: %s", stderrStr)
	}
	// The agent hint must still be surfaced on the non-terminal path.
	for _, want := range []string{"--no-wait --json", "--device-code", "lark-cli auth qrcode"} {
		if !strings.Contains(stderrStr, want) {
			t.Errorf("expected stderr agent hint to mention %q, got: %s", want, stderrStr)
		}
	}
}
