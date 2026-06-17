// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package examples

import (
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/skillscan"
)

func FromManifest(m manifest.Manifest) []skillscan.Example {
	var out []skillscan.Example
	for _, cmd := range m.Commands {
		for i, line := range strings.Split(cmd.Example, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "lark-cli ") {
				continue
			}
			out = append(out, skillscan.Example{
				Raw:            line,
				SourceFile:     "command-manifest",
				Line:           i + 1,
				HasPlaceholder: skillscan.HasPlaceholder(line),
			})
		}
	}
	return out
}
