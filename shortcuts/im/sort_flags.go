// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

type sortCompatibilityValue struct {
	legacy    string
	canonical string
}

// These tables are the single source for both the hidden legacy flag Enum and
// the value written into canonical --sort during Normalize.
var chatListSortCompatibilityValues = []sortCompatibilityValue{
	{legacy: "ByCreateTimeAsc", canonical: "create_time"},
	{legacy: "ByActiveTimeDesc", canonical: "active_time"},
}

var chatSearchSortCompatibilityValues = []sortCompatibilityValue{
	{legacy: "create_time_desc", canonical: "create_time"},
	{legacy: "update_time_desc", canonical: "update_time"},
	{legacy: "member_count_desc", canonical: "member_count"},
}

func legacySortValues(values []sortCompatibilityValue) []string {
	legacy := make([]string, 0, len(values))
	for _, value := range values {
		legacy = append(legacy, value.legacy)
	}
	return legacy
}

// legacyFlagValue preserves the precedence contract of an independently
// declared historical flag: the canonical flag wins whenever both are set.
// This is command-owned compatibility rather than a framework Flag.Alias,
// whose scalar contract is intentionally last-occurrence-wins.
func legacyFlagValue(runtime *common.RuntimeContext, legacyName, canonicalName string) (string, bool) {
	if runtime.Changed(legacyName) && !runtime.Changed(canonicalName) {
		return runtime.Str(legacyName), true
	}
	return "", false
}

// normalizeSortCompatibilityFlag is the IM adapter for the framework Normalize
// phase. It translates a legacy sort vocabulary into the canonical flag before
// framework enum validation and before Validate/DryRun/Execute. Exact name
// synonyms belong in common.Flag.Aliases and never reach this function.
func normalizeSortCompatibilityFlag(flags *common.FlagContext, legacyName, canonicalName string, values ...sortCompatibilityValue) error {
	if !flags.Changed(legacyName) {
		return nil
	}
	legacy := flags.Str(legacyName)
	if legacy == "" {
		if flags.Changed(canonicalName) {
			return nil
		}
		if err := flags.SetCanonicalFrom(legacyName, canonicalName, ""); err != nil {
			return err
		}
		return nil
	}
	allowed := legacySortValues(values)
	canonical := ""
	found := false
	for _, value := range values {
		if legacy != value.legacy {
			continue
		}
		canonical = value.canonical
		found = true
		break
	}
	if !found {
		return common.ValidationErrorf("invalid value %q for --%s, allowed: %s", legacy, legacyName, strings.Join(allowed, ", ")).
			WithParam("--" + legacyName)
	}
	if flags.Changed(canonicalName) {
		return nil
	}
	return flags.SetCanonicalFrom(legacyName, canonicalName, canonical)
}

func normalizeChatListSortCompatibility(_ context.Context, flags *common.FlagContext) error {
	return normalizeSortCompatibilityFlag(flags, "sort-type", "sort", chatListSortCompatibilityValues...)
}

func normalizeChatSearchSortCompatibility(_ context.Context, flags *common.FlagContext) error {
	return normalizeSortCompatibilityFlag(flags, "sort-by", "sort", chatSearchSortCompatibilityValues...)
}
