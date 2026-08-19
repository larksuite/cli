// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
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

// metadata returns metadata for all known domains, sorted by name.
func (r domainResolver) metadata(lang string, brand core.LarkBrand) []domainMeta {
	known := r.allKnown(brand)
	scopeless := r.scopeless()
	domains := make([]domainMeta, 0, len(known))
	for name := range known {
		if scopeless[name] {
			continue
		}
		domains = append(domains, buildDomainMeta(name, lang))
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
	// Fallback: read from the typed service spec (legacy)
	dm := domainMeta{Name: name}
	if svc, ok := registry.ServiceTyped(name); ok {
		dm.Title = svc.Title
		dm.Description = svc.Description
	}
	return dm
}

func runInteractiveLogin(ios *cmdutil.IOStreams, lang string, msg *loginMsg, brand core.LarkBrand, resolver domainResolver) (*interactiveResult, error) {
	allDomains := resolver.metadata(lang, brand)

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
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "no domains selected").WithParam("--domain")
	}

	// Compute scope summary
	scopes := resolver.scopesFor(selectedDomains, "user", brand)
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
