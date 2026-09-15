// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// allowPathsPlugin declares allow roots through the optional registrar
// extension; the install pipeline must capture them into PluginAllowPaths for
// the path policy.
type allowPathsPlugin struct {
	roots []string
	twice bool
}

func (allowPathsPlugin) Name() string    { return "pather" }
func (allowPathsPlugin) Version() string { return "1.0.0" }
func (allowPathsPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (p allowPathsPlugin) Install(r platform.Registrar) error {
	ar, ok := r.(platform.AllowPathsRegistrar)
	if !ok {
		return errors.New("host registrar does not support AllowPaths")
	}
	if p.twice {
		ar.AllowPaths(p.roots[0])
		ar.AllowPaths(p.roots[1:]...)
		return nil
	}
	ar.AllowPaths(p.roots...)
	return nil
}

func TestInstallAll_allowPathsCommitted(t *testing.T) {
	roots := []string{"/data/exports", "~/Downloads/acme"}
	result, err := internalplatform.InstallAll([]platform.Plugin{allowPathsPlugin{roots: roots}}, nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	if len(result.PluginAllowPaths) != 1 {
		t.Fatalf("PluginAllowPaths = %d, want 1", len(result.PluginAllowPaths))
	}
	got := result.PluginAllowPaths[0]
	if got.PluginName != "pather" {
		t.Errorf("PluginName = %q, want pather", got.PluginName)
	}
	if strings.Join(got.Roots, ",") != "/data/exports,~/Downloads/acme" {
		t.Errorf("Roots = %v, want the declared roots in order", got.Roots)
	}

	// The committed roots are a snapshot: the plugin mutating its own slice
	// after install must not reach the policy.
	roots[0] = "MUTATED"
	if got.Roots[0] != "/data/exports" {
		t.Errorf("committed roots alias the plugin's slice: %v", got.Roots)
	}
}

// Several AllowPaths calls by one plugin collapse into one contribution under
// that plugin's name, in call order.
func TestInstallAll_allowPathsAccumulatePerPlugin(t *testing.T) {
	result, err := internalplatform.InstallAll(
		[]platform.Plugin{allowPathsPlugin{roots: []string{"/a", "/b", "/c"}, twice: true}}, nil)
	if err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	if len(result.PluginAllowPaths) != 1 {
		t.Fatalf("PluginAllowPaths = %d, want 1", len(result.PluginAllowPaths))
	}
	if got := strings.Join(result.PluginAllowPaths[0].Roots, ","); got != "/a,/b,/c" {
		t.Errorf("Roots = %q, want /a,/b,/c", got)
	}
}

// A hand-written plugin can bypass Builder's automatic FailClosed flip. The
// staging registrar validates the actual contribution: a FailOpen plugin that
// declared allow paths is an invalid capability, not an ordinary install
// failure that FailOpen may skip.
type failOpenAllowPathsPlugin struct{}

func (failOpenAllowPathsPlugin) Name() string    { return "fail-open-pather" }
func (failOpenAllowPathsPlugin) Version() string { return "1.0.0" }
func (failOpenAllowPathsPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailOpen}
}
func (failOpenAllowPathsPlugin) Install(r platform.Registrar) error {
	r.(platform.AllowPathsRegistrar).AllowPaths("/data/exports")
	return nil
}

func TestInstallAll_failOpenAllowPathsIsFatalInvalidCapability(t *testing.T) {
	var warnings bytes.Buffer
	result, err := internalplatform.InstallAll(
		[]platform.Plugin{failOpenAllowPathsPlugin{}}, &warnings)
	if err == nil {
		t.Fatal("FailOpen+AllowPaths must abort before the roots reach the policy")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil on fatal invalid capability", result)
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonInvalidCapability {
		t.Fatalf("err = %v, want PluginInstallError reason_code %s",
			err, internalplatform.ReasonInvalidCapability)
	}
	if !strings.Contains(pi.Reason, "AllowPaths requires FailClosed") {
		t.Fatalf("reason = %q, want AllowPaths FailClosed guidance", pi.Reason)
	}
	if warnings.Len() != 0 {
		t.Fatalf("fatal mismatch was rendered as a FailOpen warning: %q", warnings.String())
	}
}

// AllowPaths() with no roots is a registration fault: a declaration that
// grants nothing is an authoring bug, reported under invalid_allow_paths.
type emptyAllowPathsPlugin struct{}

func (emptyAllowPathsPlugin) Name() string    { return "empty-pather" }
func (emptyAllowPathsPlugin) Version() string { return "1.0.0" }
func (emptyAllowPathsPlugin) Capabilities() platform.Capabilities {
	return platform.Capabilities{FailurePolicy: platform.FailClosed}
}
func (emptyAllowPathsPlugin) Install(r platform.Registrar) error {
	r.(platform.AllowPathsRegistrar).AllowPaths()
	return nil
}

func TestInstallAll_emptyAllowPathsRejected(t *testing.T) {
	result, err := internalplatform.InstallAll([]platform.Plugin{emptyAllowPathsPlugin{}}, nil)
	if err == nil {
		t.Fatal("AllowPaths() with no roots must fail the install")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil when the FailClosed plugin fails", result)
	}
	var pi *internalplatform.PluginInstallError
	if !errors.As(err, &pi) || pi.ReasonCode != internalplatform.ReasonInvalidAllowPaths {
		t.Fatalf("err = %v, want PluginInstallError reason_code %s",
			err, internalplatform.ReasonInvalidAllowPaths)
	}
}
