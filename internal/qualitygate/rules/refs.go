// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/skillscan"
)

var errUnknownCommand = errors.New("unknown command")

type ParsedExample struct {
	CommandPath string
	Flags       []string
	Positional  []string
}

type ReferencePolicy struct {
	Incremental            bool
	ChangedFiles           map[string]bool
	CommandSurfaceAffected bool
	BaseManifest           *manifest.Manifest
	BaseManifestComplete   bool
}

func CheckReferences(m manifest.Manifest, examples []skillscan.Example) ([]report.Diagnostic, []facts.SkillFact) {
	return CheckReferencesWithPolicy(m, examples, ReferencePolicy{})
}

func CheckReferencesWithPolicy(m manifest.Manifest, examples []skillscan.Example, policy ReferencePolicy) ([]report.Diagnostic, []facts.SkillFact) {
	index := indexManifest(m)
	var diags []report.Diagnostic
	var skillFacts []facts.SkillFact
	for _, ex := range examples {
		fact := facts.SkillFact{SourceFile: ex.SourceFile, Line: ex.Line, Raw: ex.Raw}
		parsed, err := parseAgainstManifest(m, ex.Raw)
		if err != nil {
			if errors.Is(err, errUnknownCommand) && commandPathContainsPlaceholder(ex.Raw) {
				skillFacts = append(skillFacts, fact)
				continue
			}
			if errors.Is(err, errUnknownCommand) {
				fact.ReferencesInvalidCommand = true
				diags = append(diags, applyReferencePolicy(rejectUnknownCommand(ex, unknownCommandPath(ex.Raw)), ex, policy))
			} else {
				diags = append(diags, parseWarning(ex, err))
			}
			skillFacts = append(skillFacts, fact)
			continue
		}
		fact.CommandPath = parsed.CommandPath
		for _, flag := range parsed.Flags {
			if index.hasFlag(parsed.CommandPath, flag) {
				continue
			}
			fact.ReferencesInvalidCommand = true
			diags = append(diags, applyReferencePolicy(rejectUnknownFlag(ex, parsed.CommandPath, flag), ex, policy))
		}
		skillFacts = append(skillFacts, fact)
	}
	return diags, skillFacts
}

func applyReferencePolicy(diag report.Diagnostic, ex skillscan.Example, policy ReferencePolicy) report.Diagnostic {
	if diag.Action != report.ActionReject || !policy.Incremental {
		return diag
	}
	sourceFile := normalizeReferencePath(ex.SourceFile)
	if !strings.HasPrefix(sourceFile, "skills/") || policy.ChangedFiles[sourceFile] {
		return diag
	}
	if referenceBecameInvalid(ex, policy) {
		return diag
	}
	diag.Action = report.ActionWarning
	diag.Suggestion = "unchanged legacy skill reference; fix in a skill-specific PR or update the changed skill file before this becomes blocking"
	return diag
}

func referenceBecameInvalid(ex skillscan.Example, policy ReferencePolicy) bool {
	if !policy.CommandSurfaceAffected {
		return false
	}
	if policy.BaseManifest == nil {
		return false
	}
	if _, err := parseAgainstManifest(*policy.BaseManifest, ex.Raw); err == nil {
		return true
	}
	return false
}

func normalizeReferencePath(value string) string {
	return strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "./")
}

func parseAgainstManifest(m manifest.Manifest, raw string) (ParsedExample, error) {
	argv, err := splitShellWords(raw)
	if err != nil {
		return ParsedExample{}, err
	}
	if len(argv) == 0 || argv[0] != "lark-cli" {
		return ParsedExample{}, fmt.Errorf("not a lark-cli command")
	}
	idx := indexManifest(m)
	for end := len(argv); end > 1; end-- {
		candidate := strings.Join(argv[1:end], " ")
		cmd := idx.commands[candidate]
		if cmd == nil {
			continue
		}
		remaining := argv[end:]
		if idx.children[candidate] && startsWithCommandToken(remaining) {
			continue
		}
		flags, positional, err := consumeFlags(remaining, cmd)
		if err != nil {
			return ParsedExample{}, err
		}
		return ParsedExample{CommandPath: candidate, Flags: flags, Positional: positional}, nil
	}
	return ParsedExample{}, errUnknownCommand
}

func commandPathContainsPlaceholder(raw string) bool {
	argv, err := splitShellWords(raw)
	if err != nil {
		return false
	}
	for _, arg := range argv[1:] {
		if isShellOperator(arg) || strings.HasPrefix(arg, "-") {
			return false
		}
		if skillscan.HasPlaceholder(arg) {
			return true
		}
	}
	return false
}

func startsWithCommandToken(args []string) bool {
	return len(args) > 0 && args[0] != "--" && !strings.HasPrefix(args[0], "-")
}

func consumeFlags(args []string, cmd *manifest.Command) ([]string, []string, error) {
	var flags []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if isShellOperator(arg) {
			break
		}
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			hasInlineValue := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
				hasInlineValue = true
			}
			flag := findManifestFlag(cmd, name)
			flags = append(flags, name)
			if flag != nil && !hasInlineValue && flag.TakesValue && i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			name := strings.TrimPrefix(arg, "-")
			if name == "h" {
				name = "help"
			}
			if flag := findManifestFlag(cmd, name); flag != nil {
				name = flag.Name
				if flag.TakesValue && i+1 < len(args) {
					i++
				}
			}
			flags = append(flags, name)
			continue
		}
		positional = append(positional, arg)
	}
	return flags, positional, nil
}

func isShellOperator(arg string) bool {
	return arg == "#" || arg == "|" || arg == "&&" || arg == "||" || arg == ">" || arg == "2>" || arg == "<"
}

func findManifestFlag(cmd *manifest.Command, name string) *manifest.Flag {
	for i := range cmd.Flags {
		if cmd.Flags[i].Name == name || cmd.Flags[i].Shorthand == name {
			return &cmd.Flags[i]
		}
	}
	return nil
}

func unknownCommandPath(raw string) string {
	argv, err := splitShellWords(raw)
	if err != nil || len(argv) <= 1 {
		return ""
	}
	var parts []string
	for _, arg := range argv[1:] {
		if strings.HasPrefix(arg, "-") {
			break
		}
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}

type manifestIndex struct {
	commands map[string]*manifest.Command
	children map[string]bool
	flags    map[string]map[string]bool
}

func indexManifest(m manifest.Manifest) manifestIndex {
	index := manifestIndex{
		commands: make(map[string]*manifest.Command, len(m.Commands)),
		children: make(map[string]bool, len(m.Commands)),
		flags:    make(map[string]map[string]bool, len(m.Commands)),
	}
	for i := range m.Commands {
		cmd := &m.Commands[i]
		index.commands[cmd.Path] = cmd
		flagSet := make(map[string]bool, len(cmd.Flags))
		for _, fl := range cmd.Flags {
			flagSet[fl.Name] = true
		}
		index.flags[cmd.Path] = flagSet
	}
	for _, cmd := range m.Commands {
		parts := strings.Fields(cmd.Path)
		for n := 1; n < len(parts); n++ {
			parent := strings.Join(parts[:n], " ")
			if index.commands[parent] != nil {
				index.children[parent] = true
			}
		}
	}
	return index
}

func (i manifestIndex) hasFlag(commandPath, flag string) bool {
	if flag == "help" {
		return true
	}
	return i.flags[commandPath][flag]
}

func splitShellWords(raw string) ([]string, error) {
	var words []string
	var b strings.Builder
	var quote rune
	escaped := false
	inWord := false
	for _, r := range raw {
		if escaped {
			b.WriteRune(r)
			escaped = false
			inWord = true
			continue
		}
		if quote != '\'' && r == '\\' {
			escaped = true
			inWord = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			inWord = true
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case unicode.IsSpace(r):
			if inWord {
				words = append(words, b.String())
				b.Reset()
				inWord = false
			}
		default:
			b.WriteRune(r)
			inWord = true
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inWord {
		words = append(words, b.String())
	}
	return words, nil
}

func parseWarning(ex skillscan.Example, err error) report.Diagnostic {
	return report.Diagnostic{
		Rule:    "skill_command_parse",
		Action:  report.ActionWarning,
		File:    ex.SourceFile,
		Line:    ex.Line,
		Message: fmt.Sprintf("cannot parse lark-cli example: %v", err),
	}
}

func rejectUnknownCommand(ex skillscan.Example, commandPath string) report.Diagnostic {
	return report.Diagnostic{
		Rule:       "skill_command_reference",
		Action:     report.ActionReject,
		File:       ex.SourceFile,
		Line:       ex.Line,
		Message:    fmt.Sprintf("example references unknown command %q", commandPath),
		Suggestion: "update the example to use a command present in the command manifest",
	}
}

func rejectUnknownFlag(ex skillscan.Example, commandPath, flag string) report.Diagnostic {
	return report.Diagnostic{
		Rule:       "skill_command_reference",
		Action:     report.ActionReject,
		File:       ex.SourceFile,
		Line:       ex.Line,
		Message:    fmt.Sprintf("example references unknown flag --%s on %s", flag, commandPath),
		Suggestion: "update the example flag or add the flag to the command implementation",
	}
}
