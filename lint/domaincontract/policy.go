// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	publicDomainsPath  = "internal/qualitygate/config/allowlists/public-domains.txt"
	fixtureDomainsPath = "internal/qualitygate/config/allowlists/fixture-domains.txt"
)

type domainPolicyEntry struct {
	Host string
	File string
	Line int
}

type domainPolicy struct {
	Public   map[string]domainPolicyEntry
	Fixtures map[string]domainPolicyEntry
}

// isReservedExampleHostname recognizes only names reserved by RFC 2606 for
// examples, testing, invalid-name examples, and localhost use. These names are
// safe source placeholders and are policy exceptions, not supported public
// endpoints.
func isReservedExampleHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	switch host {
	case "example.com", "example.net", "example.org":
		return true
	}
	labels := strings.Split(host, ".")
	switch labels[len(labels)-1] {
	case "test", "example", "invalid", "localhost":
		return true
	default:
		return false
	}
}

func loadDomainPolicy(root string) (domainPolicy, error) {
	public, err := loadDomainList(root, publicDomainsPath)
	if err != nil {
		return domainPolicy{}, err
	}
	fixtures, err := loadDomainList(root, fixtureDomainsPath)
	if err != nil {
		return domainPolicy{}, err
	}
	for host, entry := range fixtures {
		if publicEntry, ok := public[host]; ok {
			return domainPolicy{}, fmt.Errorf(
				"%s:%d: hostname %q is already listed at %s:%d",
				entry.File, entry.Line, host, publicEntry.File, publicEntry.Line,
			)
		}
	}
	return domainPolicy{Public: public, Fixtures: fixtures}, nil
}

func loadDomainList(root, rel string) (map[string]domainPolicyEntry, error) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open domain allowlist %s: %w", rel, err)
	}
	defer file.Close()

	entries := map[string]domainPolicyEntry{}
	var previous string
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		host := strings.TrimSpace(scanner.Text())
		if host == "" || strings.HasPrefix(host, "#") {
			continue
		}
		if host != strings.ToLower(host) {
			return nil, fmt.Errorf("%s:%d: hostname must be lowercase: %q", rel, line, host)
		}
		if err := validatePolicyHostname(host); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", rel, line, err)
		}
		if previous != "" && host <= previous {
			return nil, fmt.Errorf("%s:%d: hostnames must be unique and sorted: %q", rel, line, host)
		}
		entries[host] = domainPolicyEntry{Host: host, File: rel, Line: line}
		previous = host
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read domain allowlist %s: %w", rel, err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s: domain list must not be empty", rel)
	}
	return entries, nil
}

func validatePolicyHostname(host string) error {
	if len(host) > 253 || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
		return fmt.Errorf("invalid exact hostname %q", host)
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("invalid exact hostname %q", host)
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return fmt.Errorf("invalid exact hostname %q", host)
		}
	}
	if !strings.ContainsAny(labels[len(labels)-1], "abcdefghijklmnopqrstuvwxyz") {
		return fmt.Errorf("invalid exact hostname %q", host)
	}
	return nil
}
