// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

const defaultBaseURL = "https://ark.ap-southeast.bytepluses.com/api/v3"

var (
	rolloutIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
	modelIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
)

type ModelConfig struct {
	Default         string   `json:"default"`
	Allowed         []string `json:"allowed"`
	AllowedBaseURLs []string `json:"allowed_base_urls"`
}

func ParseBlockMode(value string) bool {
	return value == "true"
}

func LoadPolicy(repo string) (Policy, error) {
	data, err := vfs.ReadFile(filepath.Join(repo, "internal", "qualitygate", "config", "semantic", "policy.json"))
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return Policy{}, err
	}
	if err := validatePolicy(p); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func validatePolicy(p Policy) error {
	if p.SchemaVersion != 1 {
		return fmt.Errorf("invalid policy schema_version: %d", p.SchemaVersion)
	}
	if p.DefaultEnforcement != "observe" {
		return fmt.Errorf("invalid default_enforcement: %q", p.DefaultEnforcement)
	}
	if len(p.BlockCategories) == 0 {
		return fmt.Errorf("block_categories must not be empty")
	}
	blockCategories := map[string]bool{}
	for _, category := range p.BlockCategories {
		if !allowedCategory(category) {
			return fmt.Errorf("invalid block category: %q", category)
		}
		blockCategories[category] = true
	}
	seenGroups := map[string]bool{}
	for _, group := range p.RolloutGroups {
		if !rolloutIDPattern.MatchString(group.ID) {
			return fmt.Errorf("invalid rollout group id: %q", group.ID)
		}
		if seenGroups[group.ID] {
			return fmt.Errorf("duplicate rollout group id: %q", group.ID)
		}
		seenGroups[group.ID] = true
		if group.Enforcement != "blocking" {
			return fmt.Errorf("invalid rollout enforcement for %q: %q", group.ID, group.Enforcement)
		}
		if strings.TrimSpace(group.Owner) == "" || strings.TrimSpace(group.Reason) == "" {
			return fmt.Errorf("rollout group %q requires owner and reason", group.ID)
		}
		if len(group.Categories) == 0 {
			return fmt.Errorf("rollout group %q categories must not be empty", group.ID)
		}
		for _, category := range group.Categories {
			if !blockCategories[category] {
				return fmt.Errorf("rollout group %q category %q is outside block_categories", group.ID, category)
			}
		}
		if err := validateScopeSelector(group.Scope); err != nil {
			return fmt.Errorf("rollout group %q: %w", group.ID, err)
		}
	}
	return nil
}

func validateScopeSelector(scope ScopeSelector) error {
	for _, factKind := range scope.FactKinds {
		if !allowedFactKind(factKind) {
			return fmt.Errorf("invalid fact kind: %q", factKind)
		}
	}
	for _, source := range scope.Sources {
		switch source {
		case "builtin", "shortcut", "service":
		default:
			return fmt.Errorf("invalid source: %q", source)
		}
	}
	return nil
}

func LoadModelConfig(repo string) (ModelConfig, error) {
	data, err := vfs.ReadFile(filepath.Join(repo, "internal", "qualitygate", "config", "semantic", "models.json"))
	if err != nil {
		return ModelConfig{}, err
	}
	var cfg ModelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ModelConfig{}, err
	}
	if err := validateModelConfig(&cfg); err != nil {
		return ModelConfig{}, err
	}
	return cfg, nil
}

func validateModelConfig(cfg *ModelConfig) error {
	if cfg.Default != "" {
		return fmt.Errorf("default model is not supported; configure ARK_MODEL explicitly")
	}
	allowed := map[string]bool{}
	for _, model := range cfg.Allowed {
		if !modelIDPattern.MatchString(model) {
			return fmt.Errorf("invalid model id: %q", model)
		}
		allowed[model] = true
	}
	cfg.Allowed = sortedKeys(allowed)

	baseURLs := map[string]bool{}
	for _, raw := range cfg.AllowedBaseURLs {
		normalized, err := normalizeBaseURL(raw)
		if err != nil {
			return fmt.Errorf("invalid base URL %q: %w", raw, err)
		}
		baseURLs[normalized] = true
	}
	if len(baseURLs) == 0 {
		return fmt.Errorf("allowed_base_urls must not be empty")
	}
	defaultNormalized, err := normalizeBaseURL(defaultBaseURL)
	if err != nil {
		return err
	}
	if !baseURLs[defaultNormalized] {
		return fmt.Errorf("default base URL %q is not allowed", defaultBaseURL)
	}
	cfg.AllowedBaseURLs = sortedKeys(baseURLs)
	return nil
}

func (cfg ModelConfig) AllowsModel(model string) bool {
	for _, allowed := range cfg.Allowed {
		if model == allowed {
			return true
		}
	}
	return false
}

func IsTrustedBaseURL(raw string, cfg ModelConfig) bool {
	normalized, err := normalizeBaseURL(raw)
	if err != nil {
		return false
	}
	for _, allowed := range cfg.AllowedBaseURLs {
		allowedNormalized, err := normalizeBaseURL(allowed)
		if err == nil && normalized == allowedNormalized {
			return true
		}
	}
	return false
}

func normalizeBaseURL(raw string) (string, error) {
	if strings.TrimSpace(raw) != raw || raw == "" {
		return "", fmt.Errorf("base URL must not be blank or padded")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("scheme must be https")
	}
	if u.User != nil {
		return "", fmt.Errorf("userinfo is not allowed")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("query and fragment are not allowed")
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("host is required")
	}
	if port := u.Port(); port != "" && port != "443" {
		return "", fmt.Errorf("unexpected port %q", port)
	}
	if u.Path == "" || u.Path == "/" {
		return "", fmt.Errorf("path is required")
	}
	if strings.HasSuffix(u.Path, "/") {
		return "", fmt.Errorf("trailing slash is not allowed")
	}
	cleanPath := path.Clean(u.Path)
	if cleanPath != u.Path {
		return "", fmt.Errorf("path is not canonical")
	}
	host := strings.ToLower(u.Hostname())
	if u.Port() == "443" {
		return "https://" + host + cleanPath, nil
	}
	return "https://" + host + cleanPath, nil
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func missingFile(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
