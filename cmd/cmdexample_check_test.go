// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd_test

import "strings"

// Finding kinds.
const (
	unknownCommand = "unknown_command"
	unknownFlag    = "unknown_flag"
)

// finding is a single mismatch between an example command reference and the
// catalog.
type finding struct {
	line    int
	raw     string
	kind    string // unknownCommand | unknownFlag
	path    string // resolved command path (unknownFlag) or attempted path (unknownCommand)
	flag    string // offending flag (unknownFlag only)
	suggest string // nearest known command/flag, "" if none close
}

// checkRefs validates refs against cat and returns all mismatches in order.
func checkRefs(cat *catalog, refs []ref) []finding {
	var out []finding
	for _, r := range refs {
		path, n, ok := cat.longestPrefix(r.words)
		if !ok {
			attempted := strings.Join(r.words, " ")
			out = append(out, finding{
				line: r.line, raw: r.raw, kind: unknownCommand,
				path: attempted, suggest: cat.suggestCommand(attempted),
			})
			continue
		}
		// Leftover words after a group node are an unknown subcommand (e.g. a
		// mistyped method like "batch_modify_message"). After a leaf they are
		// positionals (e.g. "api GET /path"), so only groups trigger this.
		if n < len(r.words) && cat.isGroup(path) {
			attempted := strings.Join(r.words, " ")
			out = append(out, finding{
				line: r.line, raw: r.raw, kind: unknownCommand,
				path: attempted, suggest: cat.suggestCommand(attempted),
			})
			continue
		}
		for _, f := range r.flags {
			if cat.hasFlag(path, f) {
				continue
			}
			out = append(out, finding{
				line: r.line, raw: r.raw, kind: unknownFlag,
				path: path, flag: f, suggest: cat.suggestFlag(path, f),
			})
		}
	}
	return out
}
