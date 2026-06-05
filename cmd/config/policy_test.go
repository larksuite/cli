// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
)

func newPolicyTestFactory() (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	f := &cmdutil.Factory{
		IOStreams: cmdutil.NewIOStreams(nil, out, errOut),
	}
	return f, out, errOut
}

// `config policy show` reads the active policy recorded by bootstrap.
// When nothing is recorded the command must still produce a JSON
// envelope with source=none and a note explaining the missing context.
func TestConfigPolicyShow_NoActivePolicy(t *testing.T) {
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	f, out, _ := newPolicyTestFactory()
	if err := runConfigPolicyShow(f); err != nil {
		t.Fatalf("show: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out.String())
	}
	if got["source"] != "none" {
		t.Errorf("source = %v, want none", got["source"])
	}
	if got["note"] == "" || got["note"] == nil {
		t.Errorf("expected explanatory note when no policy recorded")
	}
}

// When bootstrap recorded an active plugin Rule, `show` emits the rule
// plus its source.
func TestConfigPolicyShow_PluginActive(t *testing.T) {
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	rule := &platform.Rule{
		Name:    "secaudit",
		Allow:   []string{"docs/**"},
		MaxRisk: "read",
	}
	cmdpolicy.SetActive(&cmdpolicy.ActivePolicy{
		Rules: []*platform.Rule{rule},
		Source: cmdpolicy.ResolveSource{
			Kind: cmdpolicy.SourcePlugin,
			Name: "secaudit",
		},
		DeniedPaths: 42,
	})

	f, out, _ := newPolicyTestFactory()
	if err := runConfigPolicyShow(f); err != nil {
		t.Fatalf("show: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out.String())
	}
	if got["source"] != "plugin" {
		t.Errorf("source = %v, want plugin", got["source"])
	}
	if got["source_name"] != "secaudit" {
		t.Errorf("source_name = %v, want secaudit", got["source_name"])
	}
	// json.Unmarshal returns float64 for numbers.
	if got["denied_paths"] != float64(42) {
		t.Errorf("denied_paths = %v, want 42", got["denied_paths"])
	}
	rulesAny, ok := got["rules"].([]any)
	if !ok || len(rulesAny) != 1 {
		t.Fatalf("rules field missing or wrong shape: %v", got["rules"])
	}
	ruleMap, ok := rulesAny[0].(map[string]any)
	if !ok {
		t.Fatalf("rules[0] wrong type")
	}
	if ruleMap["name"] != "secaudit" {
		t.Errorf("rules[0].name = %v", ruleMap["name"])
	}
}

// `source_name` must be empty when source=yaml. The yaml path is
// deliberately not surfaced (matches engine envelope convention,
// avoids leaking the user's home dir to AI agents / CI logs). The
// rule's "name:" field is the disambiguator users should rely on.
func TestConfigPolicyShow_YamlSourceNameIsEmpty(t *testing.T) {
	cmdpolicy.ResetActiveForTesting()
	t.Cleanup(cmdpolicy.ResetActiveForTesting)

	cmdpolicy.SetActive(&cmdpolicy.ActivePolicy{
		Rules: []*platform.Rule{{Name: "my-yaml-rule"}},
		Source: cmdpolicy.ResolveSource{
			Kind: cmdpolicy.SourceYAML,
			Name: "/Users/alice/.lark-cli/policy.yml",
		},
	})

	f, out, _ := newPolicyTestFactory()
	if err := runConfigPolicyShow(f); err != nil {
		t.Fatalf("show: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not json: %v\n%s", err, out.String())
	}
	if got["source"] != "yaml" {
		t.Errorf("source = %v, want yaml", got["source"])
	}
	if got["source_name"] != "" {
		t.Errorf("source_name = %q, want empty (yaml path must not leak)", got["source_name"])
	}
	// The path must not appear anywhere in the envelope.
	if bytes.Contains(out.Bytes(), []byte("/Users/alice")) {
		t.Errorf("envelope leaked yaml path: %s", out.String())
	}
}

// Regression: the parent `config` command declares a PersistentPreRunE
// that calls RequireBuiltinCredentialProvider; env credentials cause
// it to return external_provider. `config policy` is a diagnostic
// group that must not be blocked by that check. The group declares
// its own no-op PersistentPreRunE so cobra's "first walking up from
// leaf" picks ours over the config parent's.
func TestConfigPolicy_BypassesConfigParentPersistentPreRunE(t *testing.T) {
	f, _, _ := newPolicyTestFactory()
	group := NewCmdConfigPolicy(f)
	if group.PersistentPreRunE == nil {
		t.Fatal("config policy group must declare its own PersistentPreRunE to win over config parent")
	}
	if err := group.PersistentPreRunE(group, nil); err != nil {
		t.Errorf("config policy PersistentPreRunE should be no-op, got %v", err)
	}
}
