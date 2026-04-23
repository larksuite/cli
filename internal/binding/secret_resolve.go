// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"fmt"
	"os"
)

// ResolveSecretInput resolves a SecretInput to a plain-text secret string.
// This is the main dispatcher that handles all SecretInput forms:
//   - Plain string passthrough
//   - "${VAR_NAME}" env template expansion
//   - SecretRef object routing to env/file/exec sub-resolvers
//
// The getenv parameter allows injection for testing (typically os.Getenv).
// This function is only called during config bind (cold path).
func ResolveSecretInput(input SecretInput, cfg *SecretsConfig, getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	if input.IsZero() {
		return "", fmt.Errorf("appSecret is missing or empty")
	}

	// Plain string form (includes env templates)
	if input.IsPlain() {
		return resolvePlainOrTemplate(input.Plain, getenv)
	}

	// SecretRef object form
	return resolveSecretRef(input.Ref, cfg, getenv)
}

// resolvePlainOrTemplate handles plain strings and "${VAR}" templates.
func resolvePlainOrTemplate(value string, getenv func(string) string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("appSecret is empty string")
	}

	// Check for env template pattern: "${VAR_NAME}"
	matches := EnvTemplateRe.FindStringSubmatch(value)
	if matches != nil {
		varName := matches[1]
		envValue := getenv(varName)
		if envValue == "" {
			return "", fmt.Errorf("env variable %q referenced in openclaw.json is not set or empty", varName)
		}
		return envValue, nil
	}

	// Plain string: use as-is
	return value, nil
}

// resolveSecretRef dispatches a SecretRef to the appropriate sub-resolver.
func resolveSecretRef(ref *SecretRef, cfg *SecretsConfig, getenv func(string) string) (string, error) {
	// Lookup provider configuration
	providerConfig, err := LookupProvider(ref, cfg)
	if err != nil {
		return "", err
	}

	// Resolve the effective provider name once so downstream resolvers
	// (notably the exec JSON payload) see the config-defaulted value instead
	// of the unset literal on ref.Provider.
	providerName := ResolveDefaultProvider(ref, cfg)

	switch ref.Source {
	case "env":
		return resolveEnvRef(ref, providerConfig, getenv)
	case "file":
		return resolveFileRef(ref, providerConfig)
	case "exec":
		return resolveExecRef(ref, providerName, providerConfig, getenv)
	default:
		return "", fmt.Errorf("unsupported secret source %q", ref.Source)
	}
}

// resolveEnvRef handles {source:"env"} SecretRef.
func resolveEnvRef(ref *SecretRef, pc *ProviderConfig, getenv func(string) string) (string, error) {
	// Check allowlist if configured
	if len(pc.Allowlist) > 0 {
		allowed := false
		for _, name := range pc.Allowlist {
			if name == ref.ID {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("environment variable %q is not allowlisted in provider", ref.ID)
		}
	}

	value := getenv(ref.ID)
	if value == "" {
		return "", fmt.Errorf("environment variable %q is missing or empty", ref.ID)
	}
	return value, nil
}
