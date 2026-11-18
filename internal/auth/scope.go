// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import "strings"

// MissingScopes returns the elements of required that are absent from storedScope.
// storedScope is a space-separated list of granted scope strings (as stored in the token).
func MissingScopes(storedScope string, required []string) []string {
	granted := make(map[string]bool)
	for _, s := range strings.Fields(storedScope) {
		granted[s] = true
	}
	var missing []string
	for _, s := range required {
		if !isScopeSatisfied(granted, s) {
			missing = append(missing, s)
		}
	}
	return missing
}

// isScopeSatisfied reports whether the required scope is satisfied by the granted scopes.
// A scope is satisfied if it is explicitly granted, or if a broader (parent) scope
// covering it is granted (e.g. "docx:document" satisfies "docx:document:create").
func isScopeSatisfied(granted map[string]bool, required string) bool {
	if granted[required] {
		return true
	}
	for _, parent := range parentScopes(required) {
		if granted[parent] {
			return true
		}
	}
	return false
}

func parentScopes(scope string) []string {
	switch scope {
	case "docx:document:create", "docx:document:readonly", "docx:document:write_only":
		return []string{"docx:document"}
	default:
		return nil
	}
}
