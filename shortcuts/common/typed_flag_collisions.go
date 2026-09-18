// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // These diagnostics describe registration-time programmer errors surfaced through Define or Mount panics.
package common

import "fmt"

// validateTypedFlagMountPlan mirrors the flags the existing Shortcut mounter
// will add for this command. It does not create a new reserved namespace: names
// such as --json, --format, and non-high-risk --yes retain their established
// business meanings when declared as real flags.
func validateTypedFlagMountPlan(command *compiledCommand, hasFlagSchema bool, mountedRisk typedRisk) error {
	flags := legacyFlagsFromCompiled(command.fields)
	view := Shortcut{Flags: flags}

	primaryConflicts := map[string]string{
		"as":      "framework identity selection",
		"dry-run": "framework dry-run execution",
		"help":    "Cobra help",
		"jq":      "framework output filtering",
		"profile": "inherited profile selection",
	}
	if mountedRisk == typedRiskHighRiskWrite {
		primaryConflicts["yes"] = "framework high-risk confirmation"
	}
	if hasFlagSchema {
		primaryConflicts["print-schema"] = "framework complex-input introspection"
		primaryConflicts["flag-name"] = "framework complex-input introspection"
	}

	// Normalized aliases are installed after all framework flags. Unlike a real
	// business --format or --json flag, an alias cannot suppress framework flag
	// registration, so it must be checked against the final mounted long names.
	aliasConflicts := map[string]string{
		"as":      "framework identity selection",
		"dry-run": "framework dry-run execution",
		"format":  "framework or business output format",
		"jq":      "framework output filtering",
		"profile": "inherited profile selection",
		"help":    "Cobra help",
	}
	if !shortcutDeclaresJSONFlag(&view) && shortcutFormatSupportsJSON(&view) {
		aliasConflicts["json"] = "framework JSON output shorthand"
	}
	if mountedRisk == typedRiskHighRiskWrite {
		aliasConflicts["yes"] = "framework high-risk confirmation"
	}
	if hasFlagSchema {
		aliasConflicts["print-schema"] = "framework complex-input introspection"
		aliasConflicts["flag-name"] = "framework complex-input introspection"
	}

	for _, field := range command.fields {
		if reason, conflict := primaryConflicts[field.name]; conflict {
			return fmt.Errorf("Args field %s (--%s): business flag conflicts with %s flag --%s", field.goName, field.name, reason, field.name)
		}
		for _, alias := range field.cli.Aliases {
			conflicts := primaryConflicts
			kind := "independent alias"
			if alias.Mode == typedAliasNormalize {
				conflicts = aliasConflicts
				kind = "normalize alias"
			}
			if reason, conflict := conflicts[alias.Name]; conflict {
				return fmt.Errorf("Args field %s (--%s): %s --%s conflicts with %s flag --%s", field.goName, field.name, kind, alias.Name, reason, alias.Name)
			}
		}
	}
	return nil
}
