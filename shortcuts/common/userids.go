// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
)

// ResolveOpenIDs expands the special identifier "me" to the current user's
// open_id, removes duplicates case-insensitively while preserving the
// first-occurrence form, and returns nil for an empty input. flagName is
// used in error messages to point the user at the offending CLI flag.
//
// Deprecated: use ResolveOpenIDsTyped for typed error envelopes.
func ResolveOpenIDs(flagName string, ids []string, runtime *RuntimeContext) ([]string, error) {
	out, msg := resolveOpenIDs(flagName, ids, runtime)
	if msg != "" {
		return nil, output.ErrValidation("%s", msg)
	}
	return out, nil
}

// ResolveOpenIDsTyped expands the special identifier "me" to the current
// user's open_id, removes duplicates case-insensitively while preserving the
// first-occurrence form, and returns nil for an empty input. flagName names
// the flag being resolved (e.g. "--user-ids") and is recorded on the typed
// error.
func ResolveOpenIDsTyped(flagName string, ids []string, runtime *RuntimeContext) ([]string, error) {
	out, msg := resolveOpenIDs(flagName, ids, runtime)
	if msg != "" {
		return nil, ValidationErrorf("%s", msg).WithParam(flagName)
	}
	return out, nil
}

func resolveOpenIDs(flagName string, ids []string, runtime *RuntimeContext) ([]string, string) {
	if len(ids) == 0 {
		return nil, ""
	}
	currentUserID := runtime.UserOpenId()
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.EqualFold(id, "me") {
			if currentUserID == "" {
				return nil, fmt.Sprintf("%s: \"me\" requires a logged-in user with a resolvable open_id", flagName)
			}
			id = currentUserID
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out, ""
}
