// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	exttransport "github.com/larksuite/cli/extension/transport"
)

// ResolveManifestURL returns the configured distribution manifest URL. The boolean is
// false when the active transport provider does not opt into manifest-based
// distribution or returns an empty URL.
func ResolveManifestURL(ctx context.Context) (string, bool, error) {
	provider := exttransport.GetProvider()
	configured, ok := provider.(exttransport.DistributionProvider)
	if !ok {
		return "", false, nil
	}
	raw := strings.TrimSpace(configured.ResolveManifestURL(ctx))
	if raw == "" {
		return "", false, nil
	}
	if err := validateDistributionURL(raw); err != nil {
		return "", false, fmt.Errorf("invalid distribution manifest URL: %w", err)
	}
	return raw, true, nil
}

func validateDistributionURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("must be a valid URL")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain user information")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("must not contain a fragment")
	}
	return nil
}
