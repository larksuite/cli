// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	_ "embed"
	"encoding/json"
)

//go:embed service_descriptions.json
var serviceDescJSON []byte

// serviceDescLocale holds title and description for one language.
type serviceDescLocale struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// serviceDescEntry holds bilingual descriptions for a service domain.
type serviceDescEntry struct {
	En         serviceDescLocale `json:"en"`
	Zh         serviceDescLocale `json:"zh"`
	AuthDomain string            `json:"auth_domain,omitempty"`
}

var serviceDescMap map[string]serviceDescEntry

func loadServiceDescriptions() map[string]serviceDescEntry {
	if serviceDescMap != nil {
		return serviceDescMap
	}
	serviceDescMap = make(map[string]serviceDescEntry)
	_ = json.Unmarshal(serviceDescJSON, &serviceDescMap)
	return serviceDescMap
}

func getServiceLocale(name, lang string) *serviceDescLocale {
	m := loadServiceDescriptions()
	entry, ok := m[name]
	if !ok {
		return nil
	}
	if lang == "en" {
		return &entry.En
	}
	return &entry.Zh
}

// GetServiceDescription returns the localized description for a service domain,
// suitable for --help output. Returns the description field directly.
// Returns empty string if not found in the config.
func GetServiceDescription(name, lang string) string {
	loc := getServiceLocale(name, lang)
	if loc == nil {
		return ""
	}
	return loc.Description
}

// GetServiceTitle returns the localized title for a service domain.
// Returns empty string if not found.
func GetServiceTitle(name, lang string) string {
	loc := getServiceLocale(name, lang)
	if loc == nil {
		return ""
	}
	return loc.Title
}

// GetServiceDetailDescription returns the localized detail description for a service domain.
// Returns empty string if not found.
func GetServiceDetailDescription(name, lang string) string {
	loc := getServiceLocale(name, lang)
	if loc == nil {
		return ""
	}
	return loc.Description
}

// GetAuthDomain returns the auth_domain for a service, or "" if not set.
// When auth_domain is set, the service's scopes are collected under the
// parent domain during auth login.
func GetAuthDomain(service string) string {
	m := loadServiceDescriptions()
	if entry, ok := m[service]; ok {
		return entry.AuthDomain
	}
	return ""
}

// HasAuthDomain reports whether the service has an auth_domain configured.
func HasAuthDomain(service string) bool {
	return GetAuthDomain(service) != ""
}

// GetAuthChildren returns all service names whose auth_domain equals parent.
func GetAuthChildren(parent string) []string {
	m := loadServiceDescriptions()
	var children []string
	for name, entry := range m {
		if entry.AuthDomain == parent {
			children = append(children, name)
		}
	}
	return children
}
