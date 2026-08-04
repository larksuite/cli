// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package flagalias owns parse-time aliases for Cobra/pflag commands.
//
// An alias is another accepted spelling of one canonical flag. It is not a
// second pflag: parsing an alias resolves to the canonical flag before pflag
// applies the value, so type, default, Changed state, required/enum/input
// contracts, and repeated-flag behavior all stay attached to one object.
//
// Value conversion for non-equivalent legacy inputs is a business compatibility
// concern, not an alias. Exact aliases always use the canonical flag's native
// occurrence semantics; domains must not add a separate conflict policy.
package flagalias

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AnnotationAliases is attached to the canonical pflag. Consumers should use
// Aliases instead of reading the annotation directly.
const AnnotationAliases = "lark-cli/flag-aliases"

// Spec binds Aliases to one Canonical long-flag name. Names do not include the
// leading "--".
type Spec struct {
	Canonical string
	Aliases   []string
}

// Bind installs exact-name aliases on cmd and records them on their canonical
// pflags for manifest/tooling introspection. Existing pflag normalization is
// composed first; alias resolution is then applied to the normalized spelling.
// The canonical pflag also remembers the spelling that supplied its most
// recently applied value so validation errors can point back to the caller's
// actual input without changing native repeated-flag semantics.
//
// Bind is intentionally the only production owner of SetNormalizeFunc. It
// validates the complete accepted-name set before installing alias metadata or
// a normalizer, so a configuration error cannot leave aliases partially bound.
func Bind(cmd *cobra.Command, specs []Spec) error {
	if len(specs) == 0 {
		return nil
	}
	if cmd == nil {
		return fmt.Errorf("bind flag aliases: command is nil")
	}

	cmd.InitDefaultHelpFlag()
	flagSet := cmd.Flags()
	previous := flagSet.GetNormalizeFunc()
	normalize := func(name string) string {
		if previous == nil {
			return name
		}
		return string(previous(flagSet, name))
	}

	registered := make(map[string]string)
	collectRegistered(registered, flagSet)
	collectRegistered(registered, cmd.InheritedFlags())

	// Existing annotations matter when Bind is composed by multiple adapters:
	// aliases are not independent pflags, so VisitAll alone cannot see them.
	acceptedAliases := make(map[string]string)
	collectAnnotatedAliases(acceptedAliases, flagSet, normalize)
	collectAnnotatedAliases(acceptedAliases, cmd.InheritedFlags(), normalize)

	aliases := make(map[string]string)
	metadata := make(map[*pflag.Flag][]string)
	canonicalFlags := make(map[string]*pflag.Flag)
	seenCanonical := make(map[string]struct{})
	for _, spec := range specs {
		if len(spec.Aliases) == 0 {
			continue
		}
		canonicalFlag := flagSet.Lookup(spec.Canonical)
		if canonicalFlag == nil {
			return fmt.Errorf("%s declares aliases for unregistered flag --%s", cmd.CommandPath(), spec.Canonical)
		}
		canonical := canonicalFlag.Name
		if _, exists := seenCanonical[canonical]; exists {
			return fmt.Errorf("%s declares flag aliases for --%s more than once after normalization", cmd.CommandPath(), canonical)
		}
		seenCanonical[canonical] = struct{}{}
		canonicalFlags[canonical] = canonicalFlag
		for _, alias := range spec.Aliases {
			if err := validateAliasName(alias); err != nil {
				return fmt.Errorf("%s alias for --%s: %w", cmd.CommandPath(), canonical, err)
			}
			normalized := normalize(alias)
			if normalized == "" {
				return fmt.Errorf("%s alias --%s for --%s normalizes to an empty name", cmd.CommandPath(), alias, canonical)
			}
			if normalized == canonical {
				return fmt.Errorf("%s declares --%s as an alias of itself (--%s after normalization)", cmd.CommandPath(), alias, canonical)
			}
			if existing, ok := registered[normalized]; ok {
				return fmt.Errorf("%s alias --%s for --%s conflicts with registered flag --%s after normalization", cmd.CommandPath(), alias, canonical, existing)
			}
			if existing, ok := acceptedAliases[normalized]; ok {
				return fmt.Errorf("%s alias --%s for --%s conflicts with existing alias for --%s after normalization to --%s", cmd.CommandPath(), alias, canonical, existing, normalized)
			}
			if existing, ok := aliases[normalized]; ok {
				if existing == canonical {
					return fmt.Errorf("%s declares duplicate alias --%s for --%s after normalization to --%s", cmd.CommandPath(), alias, canonical, normalized)
				}
				return fmt.Errorf("%s alias --%s maps to both --%s and --%s after normalization to --%s", cmd.CommandPath(), alias, existing, canonical, normalized)
			}
			aliases[normalized] = canonical
			metadata[canonicalFlag] = append(metadata[canonicalFlag], alias)
		}
	}

	if len(aliases) == 0 {
		return nil
	}
	tracked := make(map[string]*trackedValue, len(canonicalFlags))
	for canonical, flag := range canonicalFlags {
		tracked[canonical] = ensureTrackedValue(flag)
	}
	for flag, names := range metadata {
		setAliases(flag, append(Aliases(flag), names...))
	}
	flagSet.SetNormalizeFunc(func(set *pflag.FlagSet, name string) pflag.NormalizedName {
		normalized := name
		if previous != nil {
			normalized = string(previous(set, name))
		}
		if canonical, ok := aliases[normalized]; ok {
			if set.Parsed() {
				tracked[canonical].pendingSource = name
			}
			return pflag.NormalizedName(canonical)
		}
		// Preserve the caller's raw spelling when a previously installed
		// normalizer (for example underscore-to-dash normalization) resolves it
		// directly to a canonical flag. pflag immediately normalizes the
		// canonical name once more from FlagSet.Set; leaving pendingSource intact
		// lets trackedValue.Set commit the original spelling for that occurrence.
		if value, ok := tracked[normalized]; ok && set.Parsed() && name != normalized {
			value.pendingSource = name
		}
		return pflag.NormalizedName(normalized)
	})
	return nil
}

// MustBind is the flag-registration form of Bind. Cobra/pflag registration
// already treats duplicate or invalid flag definitions as programmer errors;
// MustBind preserves that startup-fail-fast contract for callers whose mount
// API does not return an error.
func MustBind(cmd *cobra.Command, specs []Spec) {
	if err := Bind(cmd, specs); err != nil {
		panic(err)
	}
}

// Aliases returns a defensive copy of the raw accepted alias spellings stored
// on a canonical pflag. Alias order matches declaration order.
func Aliases(flag *pflag.Flag) []string {
	if flag == nil || len(flag.Annotations) == 0 {
		return nil
	}
	return append([]string(nil), flag.Annotations[AnnotationAliases]...)
}

// Source returns the spelling that supplied the canonical flag's most recently
// applied value. It returns the canonical name when no alias occurrence has
// been applied. The returned name never includes leading dashes.
func Source(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}
	if value, ok := flag.Value.(sourceValue); ok {
		if source := value.sourceName(); source != "" {
			return source
		}
	}
	return flag.Name
}

type sourceValue interface {
	pflag.Value
	sourceName() string
}

// trackedValue delegates value parsing to the canonical pflag while retaining
// only the source spelling for the last Set. pendingSource bridges pflag's two
// normalization calls for a long flag: first the caller spelling is resolved,
// then FlagSet.Set normalizes the canonical name before invoking Value.Set.
type trackedValue struct {
	pflag.Value
	canonical     string
	pendingSource string
	lastSource    string
}

func (value *trackedValue) Set(raw string) error {
	source := value.pendingSource
	if source == "" {
		source = value.canonical
	}
	value.pendingSource = ""
	// Record the attempted spelling before delegating so parse-time value
	// failures can also be attributed by callers that classify pflag errors.
	value.lastSource = source
	return value.Value.Set(raw)
}

func (value *trackedValue) sourceName() string { return value.lastSource }

// trackedSliceValue preserves pflag.SliceValue for collection flags. Without
// this adapter, wrapping a slice flag would make completion/tooling lose its
// append/replace interface even though parsing still happened to work.
type trackedSliceValue struct {
	*trackedValue
	slice pflag.SliceValue
}

func (value *trackedSliceValue) Append(raw string) error { return value.slice.Append(raw) }

func (value *trackedSliceValue) Replace(raw []string) error { return value.slice.Replace(raw) }

func (value *trackedSliceValue) GetSlice() []string { return value.slice.GetSlice() }

func ensureTrackedValue(flag *pflag.Flag) *trackedValue {
	if value, ok := flag.Value.(*trackedValue); ok {
		return value
	}
	if value, ok := flag.Value.(*trackedSliceValue); ok {
		return value.trackedValue
	}
	tracked := &trackedValue{Value: flag.Value, canonical: flag.Name}
	if slice, ok := flag.Value.(pflag.SliceValue); ok {
		flag.Value = &trackedSliceValue{trackedValue: tracked, slice: slice}
	} else {
		flag.Value = tracked
	}
	return tracked
}

func validateAliasName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("name must not be empty")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("name %q must not include leading dashes", name)
	case strings.ContainsAny(name, " \t\r\n"):
		return fmt.Errorf("name %q must not contain whitespace", name)
	case strings.Contains(name, "="):
		return fmt.Errorf("name %q must not contain '='", name)
	default:
		return nil
	}
}

func collectRegistered(dst map[string]string, set *pflag.FlagSet) {
	if set == nil {
		return
	}
	set.VisitAll(func(flag *pflag.Flag) {
		dst[flag.Name] = flag.Name
	})
}

func collectAnnotatedAliases(dst map[string]string, set *pflag.FlagSet, normalize func(string) string) {
	if set == nil {
		return
	}
	set.VisitAll(func(flag *pflag.Flag) {
		for _, alias := range Aliases(flag) {
			dst[normalize(alias)] = flag.Name
		}
	})
}

func setAliases(flag *pflag.Flag, aliases []string) {
	if flag.Annotations == nil {
		flag.Annotations = make(map[string][]string)
	}
	flag.Annotations[AnnotationAliases] = append([]string(nil), aliases...)
}
