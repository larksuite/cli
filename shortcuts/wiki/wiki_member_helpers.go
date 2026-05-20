// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"fmt"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiMemberTypes is the set of member_type values the space-member APIs
// accept. Shared by +member-add and +member-remove so the two stay aligned.
var wikiMemberTypes = []string{
	"openid", "userid", "email", "unionid", "openchat", "opendepartmentid",
}

// wikiMemberRoles is the set of member_role values the space-member APIs
// accept.
var wikiMemberRoles = []string{"admin", "member"}

// validateWikiMemberSpaceID enforces the two universal rules for the
// space-member shortcuts:
//   - --space-id must be non-blank and a valid resource name
//   - bot identity may not use the my_library alias (it has no meaning for a
//     tenant_access_token; same contract as +node-list / +node-create)
func validateWikiMemberSpaceID(runtime *common.RuntimeContext, spaceID string) error {
	if spaceID == "" {
		return output.ErrValidation("--space-id is required and cannot be blank")
	}
	if runtime.As().IsBot() && spaceID == wikiMyLibrarySpaceID {
		return output.ErrValidation("bot identity does not support --space-id my_library; use an explicit --space-id")
	}
	return validateOptionalResourceName(spaceID, "--space-id")
}

// resolveWikiMemberSpaceID transparently expands the my_library alias to the
// caller's real per-user space_id; raw IDs pass through. Mirrors the pattern
// used by +node-list so the three member shortcuts behave the same way.
func resolveWikiMemberSpaceID(runtime *common.RuntimeContext, spaceID string) (string, error) {
	if spaceID != wikiMyLibrarySpaceID {
		return spaceID, nil
	}
	resolved, err := resolveMyLibrarySpaceID(runtime)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Resolved my_library to space %s\n", common.MaskToken(resolved))
	return resolved, nil
}

// wikiMemberRecord parses a /spaces/{id}/members member object into a stable
// flat map. Used by all three shortcuts so they emit the same shape.
func wikiMemberRecord(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		// Callers (wikiMemberAddOutput, member-remove Execute) handle nil via
		// for-range or per-field fallback against the caller's input spec.
		return nil
	}
	out := map[string]interface{}{
		"member_id":   common.GetString(raw, "member_id"),
		"member_type": common.GetString(raw, "member_type"),
		"member_role": common.GetString(raw, "member_role"),
	}
	if t := common.GetString(raw, "type"); t != "" {
		out["type"] = t
	}
	return out
}
