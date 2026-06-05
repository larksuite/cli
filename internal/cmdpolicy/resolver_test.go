// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
)

func TestResolve_singlePluginWins(t *testing.T) {
	rule := &platform.Rule{Name: "secaudit"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{{PluginName: "secaudit", Rule: rule}},
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if len(got) != 1 || got[0] != rule || src.Kind != cmdpolicy.SourcePlugin || src.Name != "secaudit" {
		t.Fatalf("Resolve = (%v, %+v)", got, src)
	}
}

// A single plugin may contribute several rules (each a scoped grant). They
// are all returned, in registration order, under one plugin source.
func TestResolve_singlePluginMultipleRules(t *testing.T) {
	r1 := &platform.Rule{Name: "docs-ro", Allow: []string{"docs/**"}, MaxRisk: "read"}
	r2 := &platform.Rule{Name: "im-rw", Allow: []string{"im/**"}, MaxRisk: "write"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{
			{PluginName: "secaudit", Rule: r1},
			{PluginName: "secaudit", Rule: r2},
		},
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if len(got) != 2 || got[0] != r1 || got[1] != r2 {
		t.Fatalf("expected both rules in order, got %v", got)
	}
	if src.Kind != cmdpolicy.SourcePlugin || src.Name != "secaudit" {
		t.Fatalf("source = %+v, want plugin:secaudit", src)
	}
}

func TestResolve_pluginShadowsYaml(t *testing.T) {
	pluginRule := &platform.Rule{Name: "from-plugin"}
	yamlRule := &platform.Rule{Name: "from-yaml"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{{PluginName: "secaudit", Rule: pluginRule}},
		YAMLRules:   []*platform.Rule{yamlRule},
		YAMLPath:    "/some/policy.yml",
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "from-plugin" || src.Kind != cmdpolicy.SourcePlugin {
		t.Fatalf("plugin should shadow yaml, got %+v / %+v", got, src)
	}
}

func TestResolve_yamlWhenNoPlugin(t *testing.T) {
	yamlRule := &platform.Rule{Name: "from-yaml", MaxRisk: "read"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		YAMLRules: []*platform.Rule{yamlRule},
		YAMLPath:  "/some/policy.yml",
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "from-yaml" || src.Kind != cmdpolicy.SourceYAML {
		t.Fatalf("yaml should win when no plugin, got %+v / %+v", got, src)
	}
	if src.Name != "/some/policy.yml" {
		t.Errorf("yaml source Name should carry path, got %q", src.Name)
	}
}

// yaml may also carry several rules under "rules:"; all are returned.
func TestResolve_yamlMultipleRules(t *testing.T) {
	r1 := &platform.Rule{Name: "a", MaxRisk: "read"}
	r2 := &platform.Rule{Name: "b", MaxRisk: "write"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		YAMLRules: []*platform.Rule{r1, r2},
		YAMLPath:  "/some/policy.yml",
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if len(got) != 2 || src.Kind != cmdpolicy.SourceYAML {
		t.Fatalf("expected both yaml rules, got %v / %+v", got, src)
	}
}

func TestResolve_emptyEverythingIsNone(t *testing.T) {
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if len(got) != 0 || src.Kind != cmdpolicy.SourceNone {
		t.Fatalf("expected (empty, SourceNone), got (%v, %+v)", got, src)
	}
}

// Two DISTINCT plugins both contributing a Rule must produce the typed
// error so the bootstrap pipeline aborts (single-owner invariant): one
// plugin cannot silently widen another plugin's policy.
func TestResolve_multipleRestrictPluginsIsError(t *testing.T) {
	_, _, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{
			{PluginName: "a", Rule: &platform.Rule{Name: "a"}},
			{PluginName: "b", Rule: &platform.Rule{Name: "b"}},
		},
	})
	if !errors.Is(err, cmdpolicy.ErrMultipleRestricts) {
		t.Fatalf("err = %v, want ErrMultipleRestricts", err)
	}
}

// LoadYAMLPolicy: missing file returns (nil, nil) silently so callers
// can pass the result straight into Sources.YAMLRules without special-
// casing not-exist.
func TestLoadYAMLPolicy_missingIsSilent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent-policy.yml")
	rules, err := cmdpolicy.LoadYAMLPolicy(missing)
	if err != nil {
		t.Fatalf("missing yaml should not error, got %v", err)
	}
	if rules != nil {
		t.Fatalf("missing yaml should return nil rules, got %+v", rules)
	}
}

func TestLoadYAMLPolicy_emptyPathIsNoop(t *testing.T) {
	rules, err := cmdpolicy.LoadYAMLPolicy("")
	if err != nil {
		t.Fatalf("empty path should not error, got %v", err)
	}
	if rules != nil {
		t.Fatalf("empty path should return nil rules, got %+v", rules)
	}
}

func TestLoadYAMLPolicy_parsesValid(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "policy.yml")
	if err := os.WriteFile(yamlPath, []byte("name: from-yaml\nmax_risk: read\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	rules, err := cmdpolicy.LoadYAMLPolicy(yamlPath)
	if err != nil {
		t.Fatalf("LoadYAMLPolicy err: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "from-yaml" {
		t.Fatalf("expected one rule with name=from-yaml, got %+v", rules)
	}
}
