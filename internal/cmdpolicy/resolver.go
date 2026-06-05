// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import (
	"errors"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/platform"
	pyaml "github.com/larksuite/cli/internal/cmdpolicy/yaml"
	"github.com/larksuite/cli/internal/vfs"
)

type SourceKind string

const (
	SourcePlugin SourceKind = "plugin"
	SourceYAML   SourceKind = "yaml"
	SourceNone   SourceKind = "none"
)

type ResolveSource struct {
	Kind SourceKind
	Name string
}

type PluginRule struct {
	PluginName string
	Rule       *platform.Rule
}

type Sources struct {
	PluginRules []PluginRule
	YAMLRules   []*platform.Rule
	YAMLPath    string
}

var ErrMultipleRestricts = errors.New("multiple plugins called Restrict; only one plugin may own the policy")

// Resolve picks by precedence: plugin > yaml > none, returning the full
// rule set the winning source contributes. Pure function; load yaml via
// LoadYAMLPolicy first. Every returned rule is validated.
//
// Multi-rule semantics (single owner): one plugin may contribute several
// rules (each a scoped grant, OR-combined by the engine), but two or more
// DISTINCT plugins contributing rules is still a configuration error --
// the resolver aborts so independent plugins cannot silently widen each
// other's policy. yaml may likewise carry several rules under "rules:".
func Resolve(s Sources) ([]*platform.Rule, ResolveSource, error) {
	owners := distinctOwners(s.PluginRules)
	if len(owners) > 1 {
		return nil, ResolveSource{}, fmt.Errorf("%w: %v", ErrMultipleRestricts, owners)
	}

	if len(s.PluginRules) > 0 {
		rules := make([]*platform.Rule, 0, len(s.PluginRules))
		for _, pr := range s.PluginRules {
			if err := ValidateRule(pr.Rule); err != nil {
				return nil, ResolveSource{}, fmt.Errorf("plugin %q rule invalid: %w", pr.PluginName, err)
			}
			rules = append(rules, pr.Rule)
		}
		return rules, ResolveSource{Kind: SourcePlugin, Name: owners[0]}, nil
	}

	if len(s.YAMLRules) > 0 {
		for _, r := range s.YAMLRules {
			if err := ValidateRule(r); err != nil {
				return nil, ResolveSource{}, fmt.Errorf("policy yaml %q: %w", s.YAMLPath, err)
			}
		}
		return s.YAMLRules, ResolveSource{Kind: SourceYAML, Name: s.YAMLPath}, nil
	}

	return nil, ResolveSource{Kind: SourceNone}, nil
}

// distinctOwners returns the unique plugin names contributing a rule, in
// first-seen order. A single plugin contributing N rules collapses to one
// owner; that is the case the single-owner check below permits.
func distinctOwners(prs []PluginRule) []string {
	seen := map[string]bool{}
	owners := make([]string, 0, len(prs))
	for _, pr := range prs {
		if !seen[pr.PluginName] {
			seen[pr.PluginName] = true
			owners = append(owners, pr.PluginName)
		}
	}
	return owners
}

// LoadYAMLPolicy returns (nil, nil) when path is empty or file is absent,
// so callers can pass the result straight into Sources.YAMLRules. A
// present file yields one or more rules (see yaml.Parse).
func LoadYAMLPolicy(path string) ([]*platform.Rule, error) {
	if path == "" {
		return nil, nil
	}
	if _, err := vfs.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat policy yaml %q: %w", path, err)
	}
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy yaml %q: %w", path, err)
	}
	rules, err := pyaml.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("policy yaml %q: %w", path, err)
	}
	return rules, nil
}
