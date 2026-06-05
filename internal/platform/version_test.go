// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

func TestSatisfiesRequiredCLIVersion_constraints(t *testing.T) {
	cases := []struct {
		name       string
		build      string
		constraint string
		want       bool
		wantErr    bool
	}{
		{"empty constraint always satisfied", "1.0.0", "", true, false},
		{"DEV build always satisfied", "DEV", ">=99.0.0", true, false},
		{"empty build counts as DEV", "", ">=99.0.0", true, false},
		{"v prefix stripped", "v1.0.28", ">=1.0.0", true, false},
		{"exact match implicit operator", "1.0.0", "1.0.0", true, false},
		{"exact match explicit =", "1.0.0", "=1.0.0", true, false},
		{">= equal", "1.0.0", ">=1.0.0", true, false},
		{">= higher", "1.2.0", ">=1.0.0", true, false},
		{">= lower fails", "1.0.0", ">=2.0.0", false, false},
		{"> strict higher", "1.0.1", ">1.0.0", true, false},
		{"> equal fails", "1.0.0", ">1.0.0", false, false},
		{"<= equal", "1.0.0", "<=1.0.0", true, false},
		{"<= higher fails", "2.0.0", "<=1.0.0", false, false},
		{"< strict lower", "0.9.0", "<1.0.0", true, false},
		{"missing patch defaults to 0", "1.0", ">=1.0.0", true, false},
		{"constraint with pre-release suffix", "1.0.0-rc1", ">=1.0.0", true, false},
		{"malformed constraint returns error", "1.0.0", ">=abc", false, true},
		{"malformed constraint errors on DEV too", "DEV", ">=abc", false, true},
		{"malformed constraint errors on empty build", "", ">=zzz", false, true},
		{"unparseable build version treated as DEV", "abc", ">=1.0.0", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := satisfiesRequiredCLIVersion(tc.build, tc.constraint)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A plugin whose RequiredCLIVersion exceeds the running build must
// abort install with reason_code capability_unmet. The plugin's
// FailurePolicy then decides whether the abort bubbles up.
func TestInstallOne_RequiredCLIVersion_UnmetFailClosedAborts(t *testing.T) {
	restore := SetCurrentCLIVersionForTesting("1.0.0")
	t.Cleanup(restore)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(&capVersionPlugin{
		name:        "needs-future",
		requirement: ">=99.0.0",
		fail:        platform.FailClosed,
	})

	_, err := InstallAll(platform.RegisteredPlugins(), nil)
	if err == nil {
		t.Fatal("expected FailClosed install error, got nil")
	}
	var pi *PluginInstallError
	if !errors.As(err, &pi) {
		t.Fatalf("expected *PluginInstallError, got %T", err)
	}
	if pi.ReasonCode != ReasonCapabilityUnmet {
		t.Errorf("reason_code = %q, want %q", pi.ReasonCode, ReasonCapabilityUnmet)
	}
}

// FailOpen plugin with unmet RequiredCLIVersion is skipped (warning),
// other plugins still install.
func TestInstallOne_RequiredCLIVersion_UnmetFailOpenSkips(t *testing.T) {
	restore := SetCurrentCLIVersionForTesting("1.0.0")
	t.Cleanup(restore)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(&capVersionPlugin{
		name:        "future-failopen",
		requirement: ">=99.0.0",
		fail:        platform.FailOpen,
	})

	result, err := InstallAll(platform.RegisteredPlugins(), nil)
	if err != nil {
		t.Fatalf("FailOpen unmet must not bubble up, got: %v", err)
	}
	if result.Registry == nil {
		t.Errorf("Registry should be non-nil even after FailOpen skip")
	}
}

// A plugin authoring error in RequiredCLIVersion (parse failure) must
// abort installation UNCONDITIONALLY. Even FailOpen cannot mask a
// typo in the constraint string -- the plugin author asked the host
// to do something it cannot parse, and silently skipping would hide
// the bug from CI.
//
// Implementation: parse errors return ReasonInvalidCapability, which
// isUntrustedConfigError lists alongside restricts_mismatch so
// InstallAll's switch treats it as a hard abort.
func TestInstallOne_RequiredCLIVersion_MalformedAbortsRegardlessOfFailurePolicy(t *testing.T) {
	restore := SetCurrentCLIVersionForTesting("1.0.0")
	t.Cleanup(restore)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	// FailOpen + malformed constraint: still aborts.
	platform.Register(&capVersionPlugin{
		name:        "typo",
		requirement: ">=abc",
		fail:        platform.FailOpen,
	})

	_, err := InstallAll(platform.RegisteredPlugins(), nil)
	if err == nil {
		t.Fatal("expected malformed constraint to abort even FailOpen, got nil")
	}
	var pi *PluginInstallError
	if !errors.As(err, &pi) {
		t.Fatalf("expected *PluginInstallError, got %T", err)
	}
	if pi.ReasonCode != ReasonInvalidCapability {
		t.Errorf("reason_code = %q, want %q", pi.ReasonCode, ReasonInvalidCapability)
	}
}

// A plugin whose RequiredCLIVersion is satisfied installs normally.
func TestInstallOne_RequiredCLIVersion_SatisfiedInstalls(t *testing.T) {
	restore := SetCurrentCLIVersionForTesting("1.5.0")
	t.Cleanup(restore)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(&capVersionPlugin{
		name:        "ok",
		requirement: ">=1.0.0",
		fail:        platform.FailClosed,
	})
	if _, err := InstallAll(platform.RegisteredPlugins(), nil); err != nil {
		t.Errorf("expected install success, got %v", err)
	}
}

type capVersionPlugin struct {
	name        string
	requirement string
	fail        platform.FailurePolicy
}

func (p *capVersionPlugin) Name() string    { return p.name }
func (p *capVersionPlugin) Version() string { return "0.0.1" }
func (p *capVersionPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{
		RequiredCLIVersion: p.requirement,
		FailurePolicy:      p.fail,
	}
}
func (p *capVersionPlugin) Install(platform.Registrar) error { return nil }
