// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	qallowlist "github.com/larksuite/cli/internal/qualitygate/allowlist"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

type Allowlist map[string]string

type NamingAllowlist struct {
	Commands Allowlist
	Flags    Allowlist
}

var flagNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var commandNamePattern = regexp.MustCompile(`^\+?[a-z][a-z0-9-]*$`)

func CheckNaming(m manifest.Manifest, allow NamingAllowlist) []report.Diagnostic {
	var out []report.Diagnostic
	for _, cmd := range m.Commands {
		if cmd.Generated && cmd.Source != manifest.SourceService {
			out = append(out, report.Diagnostic{
				Rule:        "source_annotation_misuse",
				Action:      report.ActionReject,
				File:        "command-manifest",
				Message:     fmt.Sprintf("%s has generated=true but source=%s", cmd.Path, cmd.Source),
				Suggestion:  "only generated service commands may set generated=true; hand-authored commands must use builtin or shortcut source",
				SubjectType: "command",
				CommandPath: cmd.Path,
			})
			continue
		}

		var badSegments []string
		for _, part := range strings.Fields(cmd.Path) {
			if !commandNamePattern.MatchString(part) {
				badSegments = append(badSegments, part)
			}
		}
		if len(badSegments) > 0 {
			out = append(out, commandNamingDiagnostic(cmd, badSegments, allow.Commands))
		}

		for _, fl := range cmd.Flags {
			if flagNamePattern.MatchString(fl.Name) {
				continue
			}
			key := cmd.Path + " " + fl.Name
			action := report.ActionReject
			if _, ok := allow.Flags[key]; ok {
				action = report.ActionLabel
			}
			out = append(out, report.Diagnostic{
				Rule:        "flag_naming",
				Action:      action,
				File:        "command-manifest",
				Message:     fmt.Sprintf("%s --%s must use kebab-case; underscores are reserved for legacy allowlist entries", cmd.Path, fl.Name),
				Suggestion:  "use --" + strings.ReplaceAll(fl.Name, "_", "-") + " for new flags",
				SubjectType: "flag",
				CommandPath: cmd.Path,
				FlagName:    fl.Name,
			})
		}
	}
	return out
}

func commandNamingDiagnostic(cmd manifest.Command, badSegments []string, allow Allowlist) report.Diagnostic {
	action := report.ActionReject
	if allow != nil && allow[cmd.Path] != "" {
		action = report.ActionLabel
	}
	canonicalPath := cmd.CanonicalPath
	if canonicalPath == "" {
		canonicalPath = manifest.CanonicalCommandPath(cmd.Path)
	}
	return report.Diagnostic{
		Rule:        "command_naming",
		Action:      action,
		File:        "command-manifest",
		Message:     fmt.Sprintf("%s has non-kebab-case command segments: %s", cmd.Path, strings.Join(badSegments, ", ")),
		Suggestion:  fmt.Sprintf("use canonical path %q for new hand-authored commands", canonicalPath),
		SubjectType: "command",
		CommandPath: cmd.Path,
	}
}

func LoadNamingAllowlist(repo string) (NamingAllowlist, []report.Diagnostic, error) {
	commandPath := filepath.Join(repo, "internal", "qualitygate", "config", "allowlists", "legacy-commands.txt")
	commands, commandDiags, err := loadCommandAllowlist(commandPath)
	if err != nil {
		return NamingAllowlist{}, nil, err
	}
	flagPath := filepath.Join(repo, "internal", "qualitygate", "config", "allowlists", "legacy-flags.txt")
	flags, flagDiags, err := loadFlagAllowlist(flagPath)
	if err != nil {
		return NamingAllowlist{}, nil, err
	}
	diags := append(commandDiags, flagDiags...)
	return NamingAllowlist{Commands: commands, Flags: flags}, diags, nil
}

func loadCommandAllowlist(path string) (Allowlist, []report.Diagnostic, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Allowlist{}, nil, nil
		}
		return nil, nil, err
	}
	items, diags := qallowlist.ParseLegacyCommands(strings.NewReader(string(data)))
	return legacyCommandsToAllowlist(items), withAllowlistPath(diags, path), nil
}

func loadFlagAllowlist(path string) (Allowlist, []report.Diagnostic, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Allowlist{}, nil, nil
		}
		return nil, nil, err
	}
	items, diags := qallowlist.ParseLegacyFlags(strings.NewReader(string(data)))
	return legacyFlagsToAllowlist(items), withAllowlistPath(diags, path), nil
}

func legacyCommandsToAllowlist(items []qallowlist.LegacyCommand) Allowlist {
	allow := Allowlist{}
	for _, item := range items {
		allow[item.Command] = item.Owner + "\t" + item.Reason
	}
	return allow
}

func legacyFlagsToAllowlist(items []qallowlist.LegacyFlag) Allowlist {
	allow := Allowlist{}
	for _, item := range items {
		allow[item.Command+" "+item.Flag] = item.Owner + "\t" + item.Reason
	}
	return allow
}

func withAllowlistPath(diags []report.Diagnostic, path string) []report.Diagnostic {
	if len(diags) == 0 {
		return nil
	}
	out := make([]report.Diagnostic, len(diags))
	for i, diag := range diags {
		diag.File = filepath.ToSlash(path)
		out[i] = diag
	}
	return out
}

func LegacyCommandCandidates(m manifest.Manifest) []string {
	var out []string
	for _, cmd := range m.Commands {
		for _, part := range strings.Fields(cmd.Path) {
			if commandNamePattern.MatchString(part) {
				continue
			}
			out = append(out, strings.Join([]string{
				cmd.Path,
				"cli-owner",
				"legacy public command kept for compatibility",
				"2026-06-05",
			}, "\t"))
			break
		}
	}
	sort.Strings(out)
	return out
}

func LegacyFlagCandidates(m manifest.Manifest) []string {
	var out []string
	for _, cmd := range m.Commands {
		for _, fl := range cmd.Flags {
			if flagNamePattern.MatchString(fl.Name) {
				continue
			}
			out = append(out, strings.Join([]string{
				cmd.Path,
				fl.Name,
				"cli-owner",
				"legacy public flag kept for compatibility",
				"2026-06-05",
			}, "\t"))
		}
	}
	sort.Strings(out)
	return out
}
