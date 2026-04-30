// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"

	"github.com/larksuite/cli/internal/output"
)

// ResolveOpenIDs expands the special identifier "me" to the current user's
// open_id, removes duplicates case-insensitively while preserving the
// first-occurrence form, and returns nil for an empty input. flagName is
// used in error messages to point the user at the offending CLI flag.
func ResolveOpenIDs(flagName string, ids []string, runtime *RuntimeContext) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	currentUserID := runtime.UserOpenId()
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.EqualFold(id, "me") {
			if currentUserID == "" {
				return nil, output.ErrValidation("%s: \"me\" requires a logged-in user with a resolvable open_id", flagName)
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
	return out, nil
}
