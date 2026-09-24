// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// roleMemberAPIPath builds the bitable v1 path for custom-role collaborator
// management. Unlike role CRUD (which lives under /open-apis/base/v3), the
// role-member endpoints are only published on bitable v1.
func roleMemberAPIPath(baseToken, roleID, suffix string) string {
	return fmt.Sprintf("/open-apis/bitable/v1/apps/%s/roles/%s/members%s",
		validate.EncodePathSegment(baseToken), validate.EncodePathSegment(roleID), suffix)
}

// roleMemberIDTypes are the accepted --member-id-type values for add/remove.
// The batch endpoints carry the ID namespace in each member_list entry's
// `type` field (there is no member_id_type query parameter on batch calls), so
// this value is sent verbatim as the entry type.
var roleMemberIDTypes = []string{"open_id", "union_id", "user_id", "chat_id", "department_id", "open_department_id"}

// roleMemberBatchLimit is the server cap on member_list per batch call.
const roleMemberBatchLimit = 100

// roleMemberPrefixToIDType maps ID prefixes to their implied member-id-type for
// conflict validation when --member-id-type is provided explicitly. Note
// open_department_id uses a dash prefix ("od-"), not an underscore.
var roleMemberPrefixToIDType = map[string]string{
	"ou_": "open_id",
	"on_": "union_id",
	"oc_": "chat_id",
	"od-": "open_department_id",
}

// roleMemberSpec is the normalized request shared by the add/remove shortcuts.
type roleMemberSpec struct {
	BaseToken    string
	RoleID       string
	MemberIDs    []string
	MemberIDType string
}

// readRoleMemberSpec parses and validates the flags shared by the add/remove
// shortcuts.
func readRoleMemberSpec(runtime *common.RuntimeContext) (roleMemberSpec, error) {
	baseToken := strings.TrimSpace(runtime.Str("base-token"))
	if baseToken == "" {
		return roleMemberSpec{}, baseFlagErrorf("--base-token must not be blank")
	}
	roleID := strings.TrimSpace(runtime.Str("role-id"))
	if roleID == "" {
		return roleMemberSpec{}, baseFlagErrorf("--role-id must not be blank")
	}

	rawIDs := strings.TrimSpace(runtime.Str("member-ids"))
	if rawIDs == "" {
		return roleMemberSpec{}, baseFlagErrorf("--member-ids is required and cannot be blank")
	}
	memberIDs := splitRoleMemberIDs(rawIDs)
	if len(memberIDs) == 0 {
		return roleMemberSpec{}, baseFlagErrorf("--member-ids must contain at least one non-blank ID")
	}
	if len(memberIDs) > roleMemberBatchLimit {
		return roleMemberSpec{}, baseFlagErrorf("--member-ids accepts at most %d IDs per call, got %d; split into multiple batches",
			roleMemberBatchLimit, len(memberIDs))
	}
	if dup, first, second, ok := firstDuplicateRoleMemberID(memberIDs); ok {
		return roleMemberSpec{}, baseFlagErrorf("--member-ids contains duplicate ID %q at positions %d and %d; remove duplicates before retrying",
			dup, first+1, second+1)
	}

	memberIDType, err := resolveRoleMemberIDType(memberIDs, strings.TrimSpace(runtime.Str("member-id-type")))
	if err != nil {
		return roleMemberSpec{}, err
	}

	return roleMemberSpec{
		BaseToken:    baseToken,
		RoleID:       roleID,
		MemberIDs:    memberIDs,
		MemberIDType: memberIDType,
	}, nil
}

// readRoleMemberListSpec parses the list shortcut flags (no member IDs and no
// member-id-type: the list response already returns every ID namespace, and
// the list endpoint has no member_id_type parameter).
func readRoleMemberListSpec(runtime *common.RuntimeContext) (baseToken, roleID string, pageSize int, pageToken string, err error) {
	baseToken = strings.TrimSpace(runtime.Str("base-token"))
	if baseToken == "" {
		return "", "", 0, "", baseFlagErrorf("--base-token must not be blank")
	}
	roleID = strings.TrimSpace(runtime.Str("role-id"))
	if roleID == "" {
		return "", "", 0, "", baseFlagErrorf("--role-id must not be blank")
	}
	pageSize = runtime.Int("limit")
	pageToken = strings.TrimSpace(runtime.Str("page-token"))
	return baseToken, roleID, pageSize, pageToken, nil
}

// resolveRoleMemberIDType normalizes --member-id-type (default open_id) and
// rejects values whose ID-prefix implication conflicts with the declared type.
func resolveRoleMemberIDType(memberIDs []string, explicit string) (string, error) {
	memberIDType, err := normalizeRoleMemberIDType(explicit)
	if err != nil {
		return "", err
	}
	if memberIDType == "" {
		memberIDType = "open_id"
	}
	for i, id := range memberIDs {
		if implied := inferRoleMemberIDType(id); implied != "" && implied != memberIDType {
			return "", baseFlagErrorf("--member-ids entry %d %q has a prefix that implies --member-id-type %s, but %s was provided; fix the ID or pass the matching member-id-type",
				i+1, id, implied, memberIDType)
		}
	}
	return memberIDType, nil
}

func normalizeRoleMemberIDType(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	for _, candidate := range roleMemberIDTypes {
		if strings.EqualFold(raw, candidate) {
			return candidate, nil
		}
	}
	return "", baseFlagErrorf("invalid --member-id-type %q; allowed: %s", raw, strings.Join(roleMemberIDTypes, ", "))
}

func inferRoleMemberIDType(memberID string) string {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return ""
	}
	for prefix, idType := range roleMemberPrefixToIDType {
		if strings.HasPrefix(memberID, prefix) {
			return idType
		}
	}
	return ""
}

func splitRoleMemberIDs(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstDuplicateRoleMemberID(ids []string) (duplicate string, first, second int, ok bool) {
	seen := make(map[string]int, len(ids))
	for i, id := range ids {
		if prev, exists := seen[id]; exists {
			return id, prev, i, true
		}
		seen[id] = i
	}
	return "", 0, 0, false
}

// buildRoleMemberList builds the member_list body for the batch endpoints.
// Every entry carries both the ID-namespace `type` and the `id`. The batch
// endpoints expose no member_id_type query parameter, so this body `type` is
// the only thing telling the server how to interpret each id (open_id /
// union_id / user_id / chat_id / department_id / open_department_id). It is
// the ID type — not the collaborator category; the list response reports the
// category separately as member_type ("user"). Omitting the body type makes
// the endpoint report success while adding no members (silent no-op).
func buildRoleMemberList(spec roleMemberSpec) []map[string]interface{} {
	members := make([]map[string]interface{}, len(spec.MemberIDs))
	for i, id := range spec.MemberIDs {
		members[i] = map[string]interface{}{
			"type": spec.MemberIDType,
			"id":   id,
		}
	}
	return members
}

// executeRoleMemberBatch runs a batch_create / batch_delete call and reports
// the typed result. Both endpoints return only a code/msg envelope.
func executeRoleMemberBatch(runtime *common.RuntimeContext, spec roleMemberSpec, suffix, action string) error {
	body := map[string]interface{}{"member_list": buildRoleMemberList(spec)}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    roleMemberAPIPath(spec.BaseToken, spec.RoleID, suffix),
		Body:       body,
	})
	if err != nil {
		return err
	}
	return handleRoleAPIResponse(runtime, apiResp, action)
}
