// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

// GetStrFromMap extracts a string value from map[string]interface{}.
func GetStrFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetStrSliceFromMap extracts a []string value from map[string]interface{}.
// Returns nil if the key is missing or the value is not a string slice.
func GetStrSliceFromMap(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	raw, ok := m[key].([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// DeclaredScopesForMethod returns the scopes declared by a method's
// from_meta entry for the given identity. Prefers the explicit
// `requiredScopes` field when present; otherwise returns the single
// recommended scope from `scopes` (or the first scope as a final fallback).
// Returns nil when the method has no scope information.
func DeclaredScopesForMethod(method map[string]interface{}, identity string) []string {
	if method == nil {
		return nil
	}
	if requiredRaw, ok := method["requiredScopes"].([]interface{}); ok && len(requiredRaw) > 0 {
		out := make([]string, 0, len(requiredRaw))
		for _, v := range requiredRaw {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	rawScopes, _ := method["scopes"].([]interface{})
	if len(rawScopes) == 0 {
		return nil
	}
	recommended := SelectRecommendedScope(rawScopes, identity)
	if recommended == "" {
		for _, raw := range rawScopes {
			if s, ok := raw.(string); ok && s != "" {
				recommended = s
				break
			}
		}
	}
	if recommended == "" {
		return nil
	}
	return []string{recommended}
}

// SelectRecommendedScope selects the known scope with the highest priority score
// (higher = more recommended / least privilege).
// Scopes not in the priority table are skipped to avoid recommending invalid/unknown scopes.
func SelectRecommendedScope(scopes []interface{}, identity string) string {
	priorities := LoadScopePriorities()
	bestScore := -1
	bestScope := ""
	for _, s := range scopes {
		str, ok := s.(string)
		if !ok {
			continue
		}
		score, exists := priorities[str]
		if !exists {
			continue // skip unknown scopes
		}
		if score > bestScore {
			bestScore = score
			bestScope = str
		}
	}
	if bestScope != "" {
		return bestScope
	}
	// Fallback: if no scope is in the priority table, return the first one.
	if len(scopes) > 0 {
		if s, ok := scopes[0].(string); ok {
			return s
		}
	}
	return ""
}
