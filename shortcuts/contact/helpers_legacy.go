// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

// pickUserName walks a fixed list of legacy name keys returned by the older
// /contact/v3/users/{user_id} and /authen/v1/user_info endpoints. Used only
// by ContactGetUser. The newer +search-user shortcut has its own pickName
// that reads i18n_names from the v3 search response.
func pickUserName(m map[string]interface{}) string {
	for _, key := range []string{"name", "user_name", "display_name", "employee_name", "cn_name"} {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// firstNonEmpty returns the first non-empty string value among the given keys.
func firstNonEmpty(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
