// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package manifest

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/core"
)

type Source string

const (
	SourceBuiltin  Source = "builtin"
	SourceShortcut Source = "shortcut"
	SourceService  Source = "service"
)

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Commands      []Command `json:"commands"`
}

type Command struct {
	Path          string   `json:"path"`
	CanonicalPath string   `json:"canonical_path,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	Use           string   `json:"use"`
	Short         string   `json:"short,omitempty"`
	Example       string   `json:"example,omitempty"`
	Hidden        bool     `json:"hidden,omitempty"`
	Runnable      bool     `json:"runnable"`
	Source        Source   `json:"source"`
	Generated     bool     `json:"generated,omitempty"`
	Risk          string   `json:"risk,omitempty"`
	Identities    []string `json:"identities,omitempty"`
	Flags         []Flag   `json:"flags,omitempty"`
	DefaultFields []string `json:"default_fields,omitempty"`
}

type Flag struct {
	Name        string              `json:"name"`
	Aliases     []string            `json:"aliases,omitempty"`
	Shorthand   string              `json:"shorthand,omitempty"`
	Usage       string              `json:"usage,omitempty"`
	Hidden      bool                `json:"hidden,omitempty"`
	Required    bool                `json:"required,omitempty"`
	TakesValue  bool                `json:"takes_value"`
	DefValue    string              `json:"default,omitempty"`
	NoOptValue  string              `json:"no_opt_value,omitempty"`
	Annotations map[string][]string `json:"annotations,omitempty"`
}

const (
	KindCommandManifest = "command-manifest"
	KindCommandIndex    = "command-index"

	MaxManifestBytes        = 16 * 1024 * 1024
	MaxCommandsPerManifest  = 10000
	MaxFlagsPerCommand      = 200
	MaxManifestStringBytes  = 8192
	MaxAnnotationValues     = 100
	MaxAnnotationValueBytes = 8192
)

func (m Manifest) Validate(kind string) error {
	if kind != KindCommandManifest && kind != KindCommandIndex {
		return fmt.Errorf("unknown manifest kind %q", kind)
	}
	if m.SchemaVersion != 1 {
		return fmt.Errorf("%s schema_version must be 1", kind)
	}
	if len(m.Commands) == 0 {
		return fmt.Errorf("%s must contain at least one command", kind)
	}
	if len(m.Commands) > MaxCommandsPerManifest {
		return fmt.Errorf("%s has too many commands: %d", kind, len(m.Commands))
	}

	seenCommands := make(map[string]struct{}, len(m.Commands))
	hasService := false
	for i, cmd := range m.Commands {
		if err := validateCommand(kind, i, cmd); err != nil {
			return err
		}
		if _, ok := seenCommands[cmd.Path]; ok {
			return fmt.Errorf("%s command path is duplicated: %s", kind, cmd.Path)
		}
		seenCommands[cmd.Path] = struct{}{}
		if cmd.Source == SourceService {
			hasService = true
		}
	}

	switch kind {
	case KindCommandManifest:
		if hasService {
			return fmt.Errorf("%s must not contain generated service commands", kind)
		}
	case KindCommandIndex:
		if !hasService {
			return fmt.Errorf("%s must contain service commands", kind)
		}
	}
	return nil
}

func validateCommand(kind string, i int, cmd Command) error {
	prefix := fmt.Sprintf("%s commands[%d]", kind, i)
	if err := validateString(prefix+".path", cmd.Path, true); err != nil {
		return err
	}
	if err := validateString(prefix+".canonical_path", cmd.CanonicalPath, false); err != nil {
		return err
	}
	if cmd.CanonicalPath != "" && cmd.CanonicalPath != CanonicalCommandPath(cmd.Path) {
		return fmt.Errorf("%s.canonical_path = %q, want %q", prefix, cmd.CanonicalPath, CanonicalCommandPath(cmd.Path))
	}
	if err := validateString(prefix+".domain", cmd.Domain, false); err != nil {
		return err
	}
	if err := validateString(prefix+".use", cmd.Use, false); err != nil {
		return err
	}
	if err := validateString(prefix+".short", cmd.Short, false); err != nil {
		return err
	}
	if err := validateString(prefix+".example", cmd.Example, false); err != nil {
		return err
	}
	if err := validateString(prefix+".risk", cmd.Risk, false); err != nil {
		return err
	}
	// risk is a closed enum, not free text. The manifest is exported from the
	// live command tree, so this is the check that sees every mounted
	// command's real annotation — including commands whose risk arrived as a
	// string (generated service metadata) and therefore never met the Go
	// type. A misspelled level fails the gate here instead of silently
	// downgrading a destructive command to the lowest tier at runtime.
	if _, err := core.ParseRisk(cmd.Risk); err != nil {
		return fmt.Errorf("%s.risk is invalid: %q must be one of read|write|high-risk-write", prefix, cmd.Risk)
	}
	switch cmd.Source {
	case SourceBuiltin, SourceShortcut, SourceService:
	default:
		return fmt.Errorf("%s.source is invalid: %q", prefix, cmd.Source)
	}
	if cmd.Source == SourceService && !cmd.Generated {
		return fmt.Errorf("%s.generated must be true for service commands", prefix)
	}
	if cmd.Generated && cmd.Source != SourceService {
		return fmt.Errorf("%s.generated can only be true for service commands", prefix)
	}
	if len(cmd.Flags) > MaxFlagsPerCommand {
		return fmt.Errorf("%s has too many flags: %d", prefix, len(cmd.Flags))
	}
	for j, identity := range cmd.Identities {
		if err := validateString(fmt.Sprintf("%s.identities[%d]", prefix, j), identity, true); err != nil {
			return err
		}
	}
	for j, field := range cmd.DefaultFields {
		if err := validateString(fmt.Sprintf("%s.default_fields[%d]", prefix, j), field, true); err != nil {
			return err
		}
	}
	acceptedNames := make(map[string]string, len(cmd.Flags))
	for j, flag := range cmd.Flags {
		if err := validateFlag(prefix, j, flag); err != nil {
			return err
		}
		if existing, ok := acceptedNames[flag.Name]; ok {
			if existing != flag.Name {
				return fmt.Errorf("%s flags[%d].name %s conflicts with an alias of --%s", prefix, j, flag.Name, existing)
			}
			return fmt.Errorf("%s flags[%d].name is duplicated: %s", prefix, j, flag.Name)
		}
		acceptedNames[flag.Name] = flag.Name
		for k, alias := range flag.Aliases {
			if existing, ok := acceptedNames[alias]; ok {
				return fmt.Errorf("%s flags[%d].aliases[%d] %s conflicts with accepted name of --%s", prefix, j, k, alias, existing)
			}
			acceptedNames[alias] = flag.Name
		}
	}
	return nil
}

func validateFlag(commandPrefix string, i int, flag Flag) error {
	prefix := fmt.Sprintf("%s.flags[%d]", commandPrefix, i)
	if err := validateString(prefix+".name", flag.Name, true); err != nil {
		return err
	}
	if strings.ContainsAny(flag.Name, " \t\r\n") {
		return fmt.Errorf("%s.name must not contain whitespace", prefix)
	}
	seenAliases := make(map[string]struct{}, len(flag.Aliases))
	for j, alias := range flag.Aliases {
		aliasPrefix := fmt.Sprintf("%s.aliases[%d]", prefix, j)
		if err := validateString(aliasPrefix, alias, true); err != nil {
			return err
		}
		if strings.HasPrefix(alias, "-") {
			return fmt.Errorf("%s must not include leading dashes", aliasPrefix)
		}
		if strings.ContainsAny(alias, " \t\r\n") {
			return fmt.Errorf("%s must not contain whitespace", aliasPrefix)
		}
		if strings.Contains(alias, "=") {
			return fmt.Errorf("%s must not contain '='", aliasPrefix)
		}
		if alias == flag.Name {
			return fmt.Errorf("%s must differ from canonical name %s", aliasPrefix, flag.Name)
		}
		if _, ok := seenAliases[alias]; ok {
			return fmt.Errorf("%s is duplicated: %s", aliasPrefix, alias)
		}
		seenAliases[alias] = struct{}{}
	}
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "shorthand", value: flag.Shorthand},
		{name: "usage", value: flag.Usage},
		{name: "default", value: flag.DefValue},
		{name: "no_opt_value", value: flag.NoOptValue},
	} {
		if err := validateString(prefix+"."+item.name, item.value, false); err != nil {
			return err
		}
	}
	for key, values := range flag.Annotations {
		if err := validateString(prefix+".annotations key", key, true); err != nil {
			return err
		}
		if len(values) > MaxAnnotationValues {
			return fmt.Errorf("%s.annotations[%q] has too many values: %d", prefix, key, len(values))
		}
		for j, value := range values {
			if value == "" {
				return fmt.Errorf("%s.annotations[%q][%d] must not be empty", prefix, key, j)
			}
			if len(value) > MaxAnnotationValueBytes {
				return fmt.Errorf("%s.annotations[%q][%d] is too large", prefix, key, j)
			}
		}
	}
	return nil
}

func validateString(label, value string, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if len(value) > MaxManifestStringBytes {
		return fmt.Errorf("%s is too large", label)
	}
	return nil
}

func CanonicalCommandPath(path string) string {
	parts := strings.Fields(path)
	for i, part := range parts {
		prefix := ""
		if strings.HasPrefix(part, "+") {
			prefix = "+"
			part = strings.TrimPrefix(part, "+")
		}
		part = strings.ReplaceAll(part, ".", "-")
		part = strings.ReplaceAll(part, "_", "-")
		parts[i] = prefix + part
	}
	return strings.Join(parts, " ")
}
