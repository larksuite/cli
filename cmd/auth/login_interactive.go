// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
)

// domainMeta describes a domain for the interactive selector.
type domainMeta struct {
	Name        string
	Title       string
	Description string
}

// interactiveResult holds the user's selections from the interactive form.
type interactiveResult struct {
	Domains    []string
	ScopeLevel string // "common" or "all"
}

// getDomainMetadata returns metadata for all known domains, sorted by name.
func getDomainMetadata(lang string) []domainMeta {
	seen := make(map[string]bool)
	var domains []domainMeta

	// 1. Domains from from_meta projects (skip domains with auth_domain)
	for _, project := range registry.ListFromMetaProjects() {
		if registry.HasAuthDomain(project) {
			seen[project] = true
			continue
		}
		dm := buildDomainMeta(project, lang)
		domains = append(domains, dm)
		seen[project] = true
	}

	// 2. Shortcut-only domains
	shortcutOnlyNames := getShortcutOnlyDomainNames()
	for _, name := range shortcutOnlyNames {
		if !seen[name] {
			dm := buildDomainMeta(name, lang)
			domains = append(domains, dm)
			seen[name] = true
		}
	}

	// 3. Auto-discover remaining shortcut services that are listed as shortcut-only domains
	//    (skip domains with auth_domain — they are folded into their parent)
	shortcutOnlySet := make(map[string]bool)
	for _, n := range shortcutOnlyNames {
		shortcutOnlySet[n] = true
	}
	for _, sc := range shortcuts.AllShortcuts() {
		if !seen[sc.Service] {
			if shortcutOnlySet[sc.Service] && !registry.HasAuthDomain(sc.Service) {
				dm := buildDomainMeta(sc.Service, lang)
				domains = append(domains, dm)
			}
			seen[sc.Service] = true
		}
	}

	sort.Slice(domains, func(i, j int) bool {
		return domains[i].Name < domains[j].Name
	})
	return domains
}

// buildDomainMeta constructs a domainMeta for a given service name and language.
// It reads from the service_descriptions.json config first, falling back to
// from_meta spec fields if not found.
func buildDomainMeta(name, lang string) domainMeta {
	title := registry.GetServiceTitle(name, lang)
	desc := registry.GetServiceDetailDescription(name, lang)
	if title != "" || desc != "" {
		return domainMeta{
			Name:        name,
			Title:       title,
			Description: desc,
		}
	}
	// Fallback: read from from_meta spec (legacy)
	meta := registry.LoadFromMeta(name)
	dm := domainMeta{Name: name}
	if meta != nil {
		if t, ok := meta["title"].(string); ok {
			dm.Title = t
		}
		if d, ok := meta["description"].(string); ok {
			dm.Description = d
		}
	}
	return dm
}

// runInteractiveLogin shows an interactive TUI form for domain and permission selection.
func runInteractiveLogin(ios *cmdutil.IOStreams, lang string, msg *loginMsg) (*interactiveResult, error) {
	allDomains := getDomainMetadata(lang)

	// Build multi-select options
	options := make([]huh.Option[string], len(allDomains))
	for i, dm := range allDomains {
		var label string
		switch {
		case dm.Title != "" && dm.Description != "":
			label = fmt.Sprintf("%-12s %s - %s", dm.Name, dm.Title, dm.Description)
		case dm.Title != "":
			label = fmt.Sprintf("%-12s %s", dm.Name, dm.Title)
		default:
			label = fmt.Sprintf("%-12s %s", dm.Name, dm.Description)
		}
		options[i] = huh.NewOption(label, dm.Name)
	}

	var selectedDomains []string
	var permLevel string

	// Phase 1a: domain selection
	// Phase 1b: permission level (shown after domain selection completes)
	form1 := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(msg.SelectDomains).
				Description(msg.DomainHint).
				Options(options...).
				Value(&selectedDomains).
				Validate(func(s []string) error {
					if len(s) == 0 {
						return fmt.Errorf(msg.ErrNoDomain)
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.PermLevel).
				Options(
					huh.NewOption(msg.PermCommon, "common"),
					huh.NewOption(msg.PermAll, "all"),
				).
				Value(&permLevel),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form1.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return nil, output.ErrBare(1)
		}
		return nil, err
	}

	if len(selectedDomains) == 0 {
		return nil, output.ErrValidation("no domains selected")
	}

	// Compute scope summary
	scopes := collectScopesForDomains(selectedDomains, "user")
	if permLevel == "common" {
		scopes = registry.FilterAutoApproveScopes(scopes)
	}

	// Print summary
	permLabel := msg.PermAllLabel
	if permLevel == "common" {
		permLabel = msg.PermCommonLabel
	}
	fmt.Fprintf(ios.ErrOut, msg.Summary)
	fmt.Fprintf(ios.ErrOut, msg.SummaryDomains, strings.Join(selectedDomains, ", "))
	fmt.Fprintf(ios.ErrOut, msg.SummaryPerm, permLabel)
	scopePreview := strings.Join(scopes, ", ")
	if len(scopePreview) > 80 {
		scopePreview = strings.Join(scopes[:3], ", ") + ", ..."
	}
	fmt.Fprintf(ios.ErrOut, msg.SummaryScopes, len(scopes), scopePreview)

	return &interactiveResult{
		Domains:    selectedDomains,
		ScopeLevel: permLevel,
	}, nil
}
