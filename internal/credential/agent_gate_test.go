// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestAgentScopePrecheckEnabled(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"off", false},
		{"OFF", false},
		{"on", true},
		{"ON", true},
		{"1", true},
		{"true", true},
		// A negative spelling must disable, not enable. Reading "set to
		// anything" as on is how LARKSUITE_CLI_X=0 turns X on.
		{"0", false},
		{"false", false},
		{"no", false},
		// An unparsable value falls back to the documented default rather than
		// taking a command down over a typo.
		{"maybe", false},
	} {
		t.Run("value="+tt.value, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", tt.value)
			if got := agentScopePrecheckEnabled(); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("unset", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "")
		if err := os.Unsetenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK"); err != nil {
			t.Fatal(err)
		}
		if agentScopePrecheckEnabled() {
			t.Fatal("pre-check should default to off")
		}
	})
}

// TestResolveScopeMetadataForAgentSkippedWhenPrecheckOff pins that the fuse
// short-circuits before any cached scope is read.
func TestResolveScopeMetadataForAgentSkippedWhenPrecheckOff(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "off")
	var buf bytes.Buffer
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		keychainScope:   "docs:doc",
		warnOut:         &buf,
	})
	got, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Available {
		t.Errorf("got %+v, want the pre-check skipped", got)
	}
	if !strings.Contains(buf.String(), "LARKSUITE_CLI_AGENT_SCOPE_PRECHECK=off") {
		t.Errorf("stderr should name the variable that disabled the check, got: %s", buf.String())
	}
}

// TestResolveScopeMetadataForAgentPrecheckOffWarnsOncePerProvider pins the once-per-provider notice.
func TestResolveScopeMetadataForAgentPrecheckOffWarnsOncePerProvider(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "off")
	var buf bytes.Buffer
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		warnOut:         &buf,
	})
	for i := 0; i < 3; i++ {
		if _, err := p.ResolveScopeMetadata(context.Background(),
			AccessSpec{Identity: core.AsUser, AppID: "cli_a"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if n := strings.Count(buf.String(), "LARKSUITE_CLI_AGENT_SCOPE_PRECHECK=off"); n != 1 {
		t.Errorf("warning printed %d time(s) across repeated calls, want exactly once: %s", n, buf.String())
	}
}

// TestResolveScopeMetadataForNonAgentIgnoresPrecheckOff pins that the fuse does not touch UAT/TAT resolution.
func TestResolveScopeMetadataForNonAgentIgnoresPrecheckOff(t *testing.T) {
	acct := &Account{AppID: "cli_u", CredentialKind: core.CredentialUAT}
	newProvider := func() *CredentialProvider {
		return NewCredentialProvider(nil,
			&mockDefaultAcct{account: acct},
			&mockDefaultToken{result: &TokenResult{Token: "tok", Scopes: "docs:doc"}},
			nil)
	}

	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "")
	os.Unsetenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK")
	on, err := newProvider().ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_u"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "off")
	off, err := newProvider().ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_u"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if on != off {
		t.Errorf("got on=%+v off=%+v, want the switch to have no effect on a non-Agent profile", on, off)
	}
	if !off.Available || off.Scopes != "docs:doc" {
		t.Errorf("got %+v, want the ordinary UAT scope resolution unaffected", off)
	}
}

// TestPeekAgentScopeIgnoresCacheWithoutAPrincipal pins that a profile whose
// credential the CLI does not manage — an injected token, an external provider
// — never borrows a local record for the same app id. Nothing ties that record
// to the same Agent, so answering with it would pre-check one Agent's call
// against another Agent's permissions.
func TestPeekAgentScopeIgnoresCacheWithoutAPrincipal(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "on")
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "",
		keychainOpenID:  "ou_someone_else",
		keychainScope:   "docs:doc",
	})
	got, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Scopes != "" {
		t.Errorf("scopes = %q, want none: the cached record belongs to an Agent this profile is not bound to", got.Scopes)
	}
}

// TestWarnAgentScopePrecheckOffOnceSilentWhenValueDoesNotDisable pins that the
// notice names a value that actually switched the check off. "maybe" falls
// back to the default; the check is off, but not because of that value, and
// "disabled by ...=maybe" would blame a setting that did nothing.
func TestWarnAgentScopePrecheckOffOnceSilentWhenValueDoesNotDisable(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_SCOPE_PRECHECK", "maybe")
	var buf bytes.Buffer
	p := newAgentCredentialProvider(t, agentFixture{
		principalOpenID: "ou_x",
		keychainScope:   "docs:doc",
		warnOut:         &buf,
	})
	if _, err := p.ResolveScopeMetadata(context.Background(),
		AccessSpec{Identity: core.AsUser, AppID: "cli_a"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "" {
		t.Fatalf("a value that did not switch the check off must print nothing, got: %s", buf.String())
	}
}
