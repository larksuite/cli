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

// Source is the validated configured distribution source.
type Source struct {
	ManifestURL string
}

// ResolveSource returns the configured distribution source. The boolean is
// false when the active transport provider does not opt into manifest-based
// distribution or returns an empty URL.
func ResolveSource(ctx context.Context) (Source, bool, error) {
	provider := exttransport.GetProvider()
	configured, ok := provider.(exttransport.DistributionProvider)
	if !ok {
		return Source{}, false, nil
	}
	raw := strings.TrimSpace(configured.ResolveDistribution(ctx).ManifestURL)
	if raw == "" {
		return Source{}, false, nil
	}
	if err := validateDistributionURL(raw); err != nil {
		return Source{}, false, fmt.Errorf("invalid distribution manifest URL: %w", err)
	}
	return Source{ManifestURL: raw}, true, nil
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
