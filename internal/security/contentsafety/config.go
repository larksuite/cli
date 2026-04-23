// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

const configFileName = "content-safety.json"

type Config struct {
	Allowlist []string
	Rules     []rule
}

type rawConfig struct {
	Allowlist []string  `json:"allowlist"`
	Rules     []rawRule `json:"rules"`
}

type rawRule struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
}

func LoadConfig(configDir string) (*Config, error) {
	path := filepath.Join(configDir, configFileName)
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read content-safety config: %w", err)
	}
	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse content-safety config: %w", err)
	}
	rules := make([]rule, 0, len(raw.Rules))
	for _, r := range raw.Rules {
		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile rule %q pattern: %w", r.ID, err)
		}
		rules = append(rules, rule{ID: r.ID, Pattern: compiled})
	}
	return &Config{Allowlist: raw.Allowlist, Rules: rules}, nil
}

func EnsureDefaultConfig(configDir string, errOut io.Writer) error {
	path := filepath.Join(configDir, configFileName)
	if _, err := vfs.Stat(path); err == nil {
		return nil
	}
	if err := vfs.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(defaultRawConfig(), "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}
	if err := vfs.WriteFile(path, append(data, '\n'), fs.FileMode(0600)); err != nil {
		return err
	}
	fmt.Fprintf(errOut, "notice: created default content-safety config at %s\n", path)
	return nil
}

func defaultRawConfig() rawConfig {
	return rawConfig{
		Allowlist: []string{"all"},
		Rules: []rawRule{
			{
				ID:      "instruction_override",
				Pattern: `(?i)ignore\s+(all\s+|any\s+|the\s+)?(previous|prior|above|earlier)\s+(instructions?|prompts?|directives?)`,
			},
			{
				ID:      "role_injection",
				Pattern: `(?i)<\s*/?\s*(system|assistant|tool|user|developer)\s*>`,
			},
			{
				ID:      "system_prompt_leak",
				Pattern: `(?i)\b(reveal|print|show|output|display|repeat)\s+(your|the|all)\s+(system\s+|initial\s+|original\s+)?(prompt|instructions?|rules?)`,
			},
			{
				ID:      "delimiter_smuggle",
				Pattern: `<\|im_(start|end|sep)\|>|<\|endoftext\|>|###\s*(system|assistant|user)\s*:`,
			},
		},
	}
}

func IsAllowlisted(cmdPath string, allowlist []string) bool {
	for _, entry := range allowlist {
		if strings.EqualFold(entry, "all") {
			return true
		}
		if cmdPath == entry || strings.HasPrefix(cmdPath, entry+".") {
			return true
		}
	}
	return false
}
