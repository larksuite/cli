// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"io"
	"regexp"
)

// preTagRegex matches <pre ...>...</pre> blocks including those spanning multiple lines.
var preTagRegex = regexp.MustCompile(`(?s)<pre\b[^>]*>(.*?)</pre>`)

// codeTagRegex matches a <code> or <code ...> opening tag.
var codeTagRegex = regexp.MustCompile(`<code\b`)

// validatePreTags checks XML content for <pre> blocks that lack a <code> child
// element. The Lark Docs API silently drops such blocks, so we warn the user.
// Returns a list of warning messages, one per offending <pre> block.
func validatePreTags(content string) []string {
	if content == "" {
		return nil
	}

	matches := preTagRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	var warnings []string
	for i, m := range matches {
		inner := m[1]
		if !codeTagRegex.MatchString(inner) {
			warnings = append(warnings, fmt.Sprintf(
				"<pre> block #%d is missing a <code> child element; the Lark Docs API will silently drop this block. Wrap the content in <code>...</code> inside the <pre> tag.",
				i+1,
			))
		}
	}
	return warnings
}

// emitPreWarnings writes pre-tag validation warnings to the given writer (typically stderr).
func emitPreWarnings(w io.Writer, warnings []string) {
	for _, wm := range warnings {
		fmt.Fprintf(w, "warning: %s\n", wm)
	}
}
