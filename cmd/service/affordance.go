// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/meta"
)

// methodLong composes a method command's long help in one place: the
// description, the affordance guidance block (when the method has one), the
// pointer to the full schema, and the params-only addendum (params whose flag
// name is taken — paramFlagBinder.paramsOnlyHelp, "" when none). Affordance
// sits near the top so an agent sees when-to-use and few-shot examples before
// the flag list.
func methodLong(description, affordance, schemaPath, paramsOnly string) string {
	var b strings.Builder
	b.WriteString(description)
	if affordance != "" {
		b.WriteString("\n\n")
		b.WriteString(affordance)
	}
	fmt.Fprintf(&b, "\n\nView parameter definitions before calling:\n  lark-cli schema %s", schemaPath)
	b.WriteString(paramsOnly)
	return b.String()
}

// renderAffordance renders a method's affordance as a help block — when to use,
// prerequisites, and (most importantly for agents) few-shot Examples — or "" when
// the method carries no affordance. It reads the single typed model
// (meta.Method.ParsedAffordance) so the help and the envelope agree on shape.
func renderAffordance(m meta.Method) string {
	a, ok := m.ParsedAffordance()
	if !ok {
		return ""
	}

	var b strings.Builder
	bullets := func(title string, items []string) {
		var nonEmpty []string
		for _, it := range items {
			if strings.TrimSpace(it) != "" {
				nonEmpty = append(nonEmpty, it)
			}
		}
		if len(nonEmpty) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s:\n", title)
		for _, it := range nonEmpty {
			fmt.Fprintf(&b, "  • %s\n", it)
		}
	}

	bullets("When to use", a.UseWhen)
	bullets("Avoid when", a.DoNotUseWhen)
	bullets("Prerequisites", a.Prerequisites)
	if len(a.Examples) > 0 {
		var lines []string
		for _, ex := range a.Examples {
			if ex.Command == "" {
				continue
			}
			if ex.Description != "" {
				lines = append(lines, fmt.Sprintf("  • %s\n      %s", ex.Description, ex.Command))
			} else {
				lines = append(lines, fmt.Sprintf("  • %s", ex.Command))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "Examples:\n%s\n", strings.Join(lines, "\n"))
		}
	}
	bullets("Related", a.Related)

	return strings.TrimRight(b.String(), "\n")
}
