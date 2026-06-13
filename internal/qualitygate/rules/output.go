// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

func CheckDefaultOutput(m manifest.Manifest) ([]report.Diagnostic, []facts.OutputFact) {
	var diags []report.Diagnostic
	var out []facts.OutputFact
	for _, cmd := range m.Commands {
		if !cmd.Runnable || !looksLikeListCommand(cmd.Path) {
			continue
		}
		fact := facts.OutputFact{
			Command:          cmd.Path,
			Fields:           cmd.DefaultFields,
			IsList:           true,
			HasDefaultLimit:  hasBoundedDefaultLimit(cmd),
			HasFieldSelector: hasAnyFlag(cmd, "fields", "field", "field-id", "select-fields"),
			HasDecisionField: hasDecisionField(cmd.DefaultFields),
		}
		if len(cmd.DefaultFields) > 0 && (!fact.HasDefaultLimit || !fact.HasDecisionField) {
			diags = append(diags, report.Diagnostic{
				Rule:        "default_output_contract",
				Action:      report.ActionReject,
				File:        "command-manifest",
				Message:     cmd.Path + " default output must include a default limit and agent decision fields",
				Suggestion:  "add a default page-size/page-limit and include fields such as id, name, status, url, or time in default output",
				SubjectType: "output",
				CommandPath: cmd.Path,
			})
		}
		if !fact.HasDefaultLimit {
			diags = append(diags, report.Diagnostic{
				Rule:        "default_output",
				Action:      report.ActionWarning,
				File:        "command-manifest",
				Message:     cmd.Path + " looks like a list command without an explicit default limit flag",
				Suggestion:  "add a default page-size/page-limit or document why the command is bounded",
				SubjectType: "output",
				CommandPath: cmd.Path,
			})
		}
		out = append(out, fact)
	}
	return diags, out
}

func looksLikeListCommand(path string) bool {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return false
	}
	last := parts[len(parts)-1]
	return last == "list" ||
		last == "search" ||
		strings.HasSuffix(last, "-list") ||
		strings.HasSuffix(last, "-search") ||
		strings.HasSuffix(last, "_list") ||
		strings.HasSuffix(last, "_search")
}

func hasAnyFlag(cmd manifest.Command, names ...string) bool {
	for _, fl := range cmd.Flags {
		for _, name := range names {
			if fl.Name == name {
				return true
			}
		}
	}
	return false
}

func hasBoundedDefaultLimit(cmd manifest.Command) bool {
	for _, fl := range cmd.Flags {
		switch fl.Name {
		case "page-size", "page-limit", "limit", "max":
			if fl.DefValue != "" && fl.DefValue != "0" {
				return true
			}
		}
	}
	return false
}

var decisionFieldNames = []string{"id", "name", "status", "url", "time", "created_at", "updated_at", "message_id", "file_token"}

func hasDecisionField(fields []string) bool {
	want := make(map[string]bool, len(decisionFieldNames))
	for _, name := range decisionFieldNames {
		want[name] = true
	}
	for _, field := range fields {
		normalized := normalizeFieldName(field)
		if want[normalized] {
			return true
		}
		for _, part := range strings.FieldsFunc(normalized, func(r rune) bool { return r == '_' }) {
			if want[part] {
				return true
			}
		}
	}
	return false
}

func normalizeFieldName(field string) string {
	normalized := strings.ToLower(field)
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_", " ", "_")
	return replacer.Replace(normalized)
}
