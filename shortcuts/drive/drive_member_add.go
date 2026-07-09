// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveMemberAddIDTypes covers every user-facing --member-type value accepted
// by the shortcut. Some values are normalized before hitting the API.
var driveMemberAddIDTypes = []string{
	"email", "openid", "unionid", "openchat", "opendepartmentid",
	"groupid", "appid", "wikispaceid",
}

var driveMemberAddPerms = []string{"view", "edit", "full_access"}
var driveMemberAddPermTypes = []string{"container", "single_page"}
var driveMemberAddWikiSpaceMemberKinds = []string{"wiki_space_member", "wiki_space_viewer", "wiki_space_editor"}

// driveMemberAddPrefixToType maps ID prefixes to their expected member_type
// for conflict validation when --member-type is provided explicitly.
var driveMemberAddPrefixToType = map[string]string{
	"ou_": "openid",
	"on_": "unionid",
	"oc_": "openchat",
	"od_": "opendepartmentid",
}

var driveMemberAddURLPathToType = []struct {
	Prefix string
	Type   string
}{
	{"/drive/folder/", "folder"},
	{"/docx/", "docx"},
	{"/doc/", "doc"},
	{"/sheets/", "sheet"},
	{"/base/", "bitable"},
	{"/bitable/", "bitable"},
	{"/wiki/", "wiki"},
	{"/file/", "file"},
	{"/mindnotes/", "mindnote"},
	{"/slides/", "slides"},
	{"/minutes/", "minutes"},
}

var driveMemberAddResourceTypes = []string{"docx", "doc", "sheet", "bitable", "file", "folder", "wiki", "mindnote", "slides", "minutes"}

const driveMemberAddBatchLimit = 10

// DriveMemberAdd adds a collaborator/member permission to a Drive resource.
var DriveMemberAdd = common.Shortcut{
	Service:     "drive",
	Command:     "+member-add",
	Description: "Add a collaborator/member permission to a Drive document, file, folder, or wiki node",
	Risk:        "high-risk-write",
	Scopes:      []string{"docs:permission.member:create"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "token", Desc: "target token or document URL; type is auto-inferred from URL path when omitted", Required: true},
		{Name: "type", Desc: "target resource type; required when --token is a bare token"},
		{Name: "member-id", Desc: "collaborator ID; comma-separated for batch (max 10). Interpretation is decided by --member-type", Required: true},
		{Name: "member-type", Desc: "ID type for --member-id; supported: email|openid|unionid|openchat|opendepartmentid|groupid|appid|wikispaceid", Required: true},
		{Name: "member-kind", Desc: "request body type when --member-type=wikispaceid; one of wiki_space_member|wiki_space_viewer|wiki_space_editor"},
		{Name: "perm", Desc: "permission role to grant; defaults to view"},
		{Name: "perm-type", Desc: "wiki permission scope; defaults to container; rejected for non-wiki types"},
		{Name: "need-notification", Type: "bool", Desc: "send an in-app notification after the grant (user identity only)"},
	},
	Tips: []string{
		"Resource type is auto-inferred from URL paths; pass --type when --token is a bare token.",
		"Supported --member-type values: email, openid, unionid, openchat, opendepartmentid, groupid, appid, wikispaceid.",
		"When --member-type=wikispaceid, pass --member-kind wiki_space_member, wiki_space_viewer, or wiki_space_editor.",
		"--member-type is required; if the ID prefix conflicts with --member-type (e.g. ou_xxx with email), the command rejects it.",
		"--perm defaults to view (safest); use --dry-run first when granting edit or full_access.",
		"For wiki nodes, --perm-type defaults to container (current page and sub-pages), except --member-type=wikispaceid where --member-kind provides the wiki-space role.",
		"Department collaborator (--member-type=opendepartmentid) requires --as user; bot identity is not supported for department authorization.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveMemberAddSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveMemberAddSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveMemberAddDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveMemberAddSpec(runtime)
		if err != nil {
			return err
		}

		if len(spec.MemberIDs) == 1 {
			return executeDriveMemberAddSingle(runtime, spec)
		}
		return executeDriveMemberAddBatch(runtime, spec)
	},
}

// driveMemberAddSpec is the normalized request model shared by Validate,
// DryRun, Execute, and output shaping so they all observe the same defaults.
type driveMemberAddSpec struct {
	Token        string
	ResourceType string
	MemberIDs    []string
	MemberType   string
	// MemberKind is the explicit --member-kind value for member_type=wikispaceid.
	MemberKind       string
	Perm             string
	PermType         string
	NeedNotification bool
	NotificationSet  bool
}

// DryRunParams builds the preview query string while preserving the semantic
// difference between an omitted notification flag and an explicit false.
func (spec driveMemberAddSpec) DryRunParams() map[string]interface{} {
	params := map[string]interface{}{"type": spec.ResourceType}
	if spec.NotificationSet {
		params["need_notification"] = spec.NeedNotification
	}
	return params
}

// APIQueryParams builds the query params for permission.members.create.
func (spec driveMemberAddSpec) APIQueryParams() map[string]interface{} {
	params := map[string]interface{}{"type": spec.ResourceType}
	if spec.NotificationSet {
		params["need_notification"] = strconv.FormatBool(spec.NeedNotification)
	}
	return params
}

// buildMemberBody builds a single member object for the request body.
func buildMemberBody(memberID, memberType, wikiSpaceMemberKind, perm, permType string) map[string]interface{} {
	body := map[string]interface{}{
		"member_id":   memberID,
		"member_type": memberType,
		"perm":        perm,
	}
	if bodyType := driveMemberAddBodyType(memberType, wikiSpaceMemberKind); bodyType != "" {
		body["type"] = bodyType
	}
	if permType != "" {
		body["perm_type"] = permType
	}
	return body
}

// readDriveMemberAddSpec parses runtime flags into a normalized request model,
// applying inference, defaults, and cross-field validation in one place.
func readDriveMemberAddSpec(runtime *common.RuntimeContext) (driveMemberAddSpec, error) {
	token, resourceType, err := resolveDriveMemberAddTarget(runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveMemberAddSpec{}, err
	}

	// Parse member-id: comma-separated for batch.
	rawMemberID := strings.TrimSpace(runtime.Str("member-id"))
	if rawMemberID == "" {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-id is required and cannot be blank").WithParam("--member-id")
	}
	memberIDs := splitAndTrimMembers(rawMemberID)
	if len(memberIDs) == 0 {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-id is required and must contain at least one non-blank ID").WithParam("--member-id")
	}
	if len(memberIDs) > driveMemberAddBatchLimit {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-id accepts at most %d IDs, got %d", driveMemberAddBatchLimit, len(memberIDs)).WithParam("--member-id")
	}
	if duplicate, first, second, ok := firstDuplicateDriveMemberID(memberIDs); ok {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--member-id contains duplicate collaborator ID %q at positions %d and %d; remove duplicates before retrying",
			duplicate, first+1, second+1,
		).WithParam("--member-id")
	}

	memberType, err := resolveDriveMemberAddMemberType(memberIDs, runtime.Str("member-type"))
	if err != nil {
		return driveMemberAddSpec{}, err
	}
	memberKind, err := resolveDriveMemberAddMemberKind(memberType, runtime.Str("member-kind"))
	if err != nil {
		return driveMemberAddSpec{}, err
	}

	// perm: default to view.
	perm, err := normalizeDriveMemberAddEnumValue(runtime.Str("perm"), driveMemberAddPerms, "--perm")
	if err != nil {
		return driveMemberAddSpec{}, err
	}
	if perm == "" {
		perm = "view"
	}

	// perm-type: only meaningful for wiki; default container except for wiki-space collaborators.
	permType, err := normalizeDriveMemberAddEnumValue(runtime.Str("perm-type"), driveMemberAddPermTypes, "--perm-type")
	if err != nil {
		return driveMemberAddSpec{}, err
	}
	if resourceType == "wiki" && memberType == "wikispaceid" {
		if runtime.Changed("perm-type") {
			return driveMemberAddSpec{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--perm-type is not supported when --member-type=wikispaceid; use --member-kind wiki_space_member|wiki_space_viewer|wiki_space_editor to set the wiki-space role",
			).WithParam("--perm-type")
		}
		permType = ""
	} else if resourceType == "wiki" && permType == "" {
		permType = driveMemberAddDefaultPermType(resourceType)
	} else if resourceType != "wiki" && runtime.Changed("perm-type") {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--perm-type only applies when resource type is wiki; got %q", resourceType).WithParam("--perm-type")
	} else if resourceType != "wiki" {
		permType = ""
	}

	spec := driveMemberAddSpec{
		Token:            token,
		ResourceType:     resourceType,
		MemberIDs:        memberIDs,
		MemberType:       memberType,
		MemberKind:       memberKind,
		Perm:             perm,
		PermType:         permType,
		NeedNotification: runtime.Bool("need-notification"),
		NotificationSet:  runtime.Changed("need-notification"),
	}
	if runtime.As().IsBot() && spec.NotificationSet {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--need-notification is only valid with --as user; omit it when using --as bot").WithParam("--need-notification")
	}
	if runtime.As().IsBot() && spec.MemberType == "opendepartmentid" {
		return driveMemberAddSpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-type=opendepartmentid requires --as user; bot identity does not support adding department collaborators").WithParam("--member-type")
	}
	return spec, nil
}

// resolveDriveMemberAddTarget extracts (token, type) from a user-supplied
// --token value that may be either a bare token or a full resource URL, plus an
// optional explicit --type. Explicit --type wins over URL inference.
func resolveDriveMemberAddTarget(raw, explicitType string) (token, resourceType string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token is required").WithParam("--token")
	}
	explicitType = strings.ToLower(strings.TrimSpace(explicitType))

	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Hostname() == "" {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token URL is malformed: %q", raw).WithParam("--token")
		}
		urlToken, urlType, ok := parseDriveMemberAddResourceURLPath(parsed.Path)
		if !ok {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unsupported URL path %q: expected one of %s followed by a token",
				parsed.Path, strings.Join(driveMemberAddSupportedURLPaths(), ", "),
			).WithParam("--token")
		}
		token = urlToken
		if explicitType == "" {
			resourceType = urlType
		}
	} else {
		token = raw
	}

	if explicitType != "" {
		if !isSupportedDriveMemberAddResourceType(explicitType) {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--type must be one of: %s", strings.Join(driveMemberAddResourceTypes, ", ")).WithParam("--type")
		}
		resourceType = explicitType
	}

	if resourceType == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--type is required when --token is a bare token; accepted values: %s",
			strings.Join(driveMemberAddResourceTypes, ", "),
		).WithParam("--type")
	}
	return token, resourceType, nil
}

func driveMemberAddSupportedURLPaths() []string {
	paths := make([]string, 0, len(driveMemberAddURLPathToType))
	for _, mapping := range driveMemberAddURLPathToType {
		paths = append(paths, mapping.Prefix)
	}
	return paths
}

func parseDriveMemberAddResourceURLPath(path string) (token, resourceType string, ok bool) {
	for _, mapping := range driveMemberAddURLPathToType {
		if !strings.HasPrefix(path, mapping.Prefix) {
			continue
		}
		token := path[len(mapping.Prefix):]
		token = strings.TrimRight(token, "/")
		if idx := strings.IndexByte(token, '/'); idx >= 0 {
			token = token[:idx]
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return "", "", false
		}
		return token, mapping.Type, true
	}
	return "", "", false
}

func isSupportedDriveMemberAddResourceType(resourceType string) bool {
	switch resourceType {
	case "docx", "doc", "sheet", "bitable", "file", "folder", "wiki", "mindnote", "slides", "minutes":
		return true
	default:
		return false
	}
}

func resolveDriveMemberAddMemberType(memberIDs []string, explicit string) (string, error) {
	var err error
	explicit, err = normalizeDriveMemberAddEnumValue(explicit, driveMemberAddIDTypes, "--member-type")
	if err != nil {
		return "", err
	}
	if explicit == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-type is required; accepted values: %s", strings.Join(driveMemberAddIDTypes, ", ")).WithParam("--member-type")
	}
	for i, memberID := range memberIDs {
		if expected := inferMemberTypeFromID(memberID); expected != "" && expected != explicit {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"member-id[%d] %q prefix implies --member-type %s, but --member-type %s was provided; fix the ID or use the matching member type",
				i+1, memberID, expected, explicit,
			).WithParam("--member-id")
		}
	}
	return normalizeDriveMemberAddMemberType(explicit), nil
}

func resolveDriveMemberAddMemberKind(memberType, raw string) (string, error) {
	memberKind, err := normalizeDriveMemberAddEnumValue(raw, driveMemberAddWikiSpaceMemberKinds, "--member-kind")
	if err != nil {
		return "", err
	}
	if memberType == "wikispaceid" {
		if memberKind == "" {
			return "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--member-kind is required when --member-type=wikispaceid; allowed: %s",
				strings.Join(driveMemberAddWikiSpaceMemberKinds, ", "),
			).WithParam("--member-kind")
		}
		return memberKind, nil
	}
	if memberKind != "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--member-kind only applies when --member-type=wikispaceid").WithParam("--member-kind")
	}
	return "", nil
}

func normalizeDriveMemberAddMemberType(memberType string) string {
	return strings.ToLower(strings.TrimSpace(memberType))
}

func normalizeDriveMemberAddEnumValue(raw string, allowed []string, flagName string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return candidate, nil
		}
	}
	return "", errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"invalid value %q for %s, allowed: %s",
		value,
		flagName,
		strings.Join(allowed, ", "),
	).WithParam(flagName)
}

// splitAndTrimMembers splits a comma-separated member-id string and trims whitespace.
func splitAndTrimMembers(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func firstDuplicateDriveMemberID(memberIDs []string) (duplicate string, first, second int, ok bool) {
	seen := make(map[string]int, len(memberIDs))
	for i, memberID := range memberIDs {
		if prev, exists := seen[memberID]; exists {
			return memberID, prev, i, true
		}
		seen[memberID] = i
	}
	return "", 0, 0, false
}

// inferMemberTypeFromID returns the expected member_type for a member-id
// based on its prefix, or "" if no prefix matches (e.g. groupid).
func inferMemberTypeFromID(memberID string) string {
	memberID = strings.TrimSpace(memberID)
	if memberID == "" {
		return ""
	}
	if strings.Contains(memberID, "@") {
		return "email"
	}
	for prefix, mtype := range driveMemberAddPrefixToType {
		if strings.HasPrefix(memberID, prefix) {
			return mtype
		}
	}
	return ""
}

// driveMemberAddDefaultPermType returns the default perm_type for a given
// resource type. For wiki nodes, container is the default for regular
// collaborators. Wiki-space collaborators omit perm_type because their role is
// carried by the body type field.
func driveMemberAddDefaultPermType(resourceType string) string {
	switch resourceType {
	case "wiki":
		return "container"
	default:
		return ""
	}
}

// inferDriveMemberKind derives the request-body collaborator kind from
// member-type for all supported member-type values.
func inferDriveMemberKind(memberType string) string {
	switch memberType {
	case "email", "openid", "unionid", "userid":
		return "user"
	case "openchat":
		return "chat"
	case "opendepartmentid":
		return "department"
	case "groupid":
		return "group"
	default:
		return ""
	}
}

func driveMemberAddBodyType(memberType, wikiSpaceMemberKind string) string {
	if memberType == "wikispaceid" {
		return wikiSpaceMemberKind
	}
	return inferDriveMemberKind(memberType)
}

// buildDriveMemberAddDryRun renders the exact request preview for --dry-run.
func buildDriveMemberAddDryRun(spec driveMemberAddSpec) *common.DryRunAPI {
	if len(spec.MemberIDs) == 1 {
		body := buildMemberBody(spec.MemberIDs[0], spec.MemberType, spec.MemberKind, spec.Perm, spec.PermType)
		return common.NewDryRunAPI().
			Desc("Add Drive collaborator/member permission").
			POST(fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members", validate.EncodePathSegment(spec.Token))).
			Params(spec.DryRunParams()).
			Body(body)
	}

	members := buildDriveMemberAddMemberBodies(spec)
	return common.NewDryRunAPI().
		Desc("Batch add Drive collaborator/member permissions").
		POST(fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members/batch_create", validate.EncodePathSegment(spec.Token))).
		Params(spec.DryRunParams()).
		Body(map[string]interface{}{"members": members})
}

// executeDriveMemberAddSingle calls the single-member create API.
func executeDriveMemberAddSingle(runtime *common.RuntimeContext, spec driveMemberAddSpec) error {
	fmt.Fprintf(runtime.IO().ErrOut, "Adding Drive member %s (type=%s, perm=%s) to %s %s...\n",
		common.MaskToken(spec.MemberIDs[0]), spec.MemberType, spec.Perm, spec.ResourceType, common.MaskToken(spec.Token))

	body := buildMemberBody(spec.MemberIDs[0], spec.MemberType, spec.MemberKind, spec.Perm, spec.PermType)
	data, err := runtime.CallAPITyped(
		"POST",
		fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members", validate.EncodePathSegment(spec.Token)),
		spec.APIQueryParams(),
		body,
	)
	if err != nil {
		return err
	}

	out := driveMemberAddOutput(spec, spec.MemberIDs[0], common.GetMap(data, "member"))
	fmt.Fprintf(runtime.IO().ErrOut, "Added Drive member %s\n", common.MaskToken(common.GetString(out, "member_id")))
	runtime.Out(out, nil)
	return nil
}

// executeDriveMemberAddBatch calls the batch_create API. A successful HTTP/API
// response is treated as complete only when the server returns every requested
// member_id, regardless of response array order.
func executeDriveMemberAddBatch(runtime *common.RuntimeContext, spec driveMemberAddSpec) error {
	members := buildDriveMemberAddMemberBodies(spec)

	fmt.Fprintf(runtime.IO().ErrOut, "Adding %d Drive members (type=%s, perm=%s) to %s %s...\n",
		len(spec.MemberIDs), spec.MemberType, spec.Perm, spec.ResourceType, common.MaskToken(spec.Token))

	data, err := runtime.CallAPITyped(
		"POST",
		fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members/batch_create", validate.EncodePathSegment(spec.Token)),
		spec.APIQueryParams(),
		map[string]interface{}{"members": members},
	)
	if err != nil {
		return wrapDriveMemberAddBatchAPIError(err)
	}

	result := buildDriveMemberAddBatchResult(spec, data)
	if common.GetBool(result, "partial") {
		return runtime.OutPartialFailure(result, nil)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Added %d Drive member(s)\n", result["succeeded_count"])
	runtime.Out(result, nil)
	return nil
}

const (
	driveMemberAddInvalidParameterCode = 1063001
	driveMemberAddInvalidOperationCode = 1063003
)

func wrapDriveMemberAddBatchAPIError(err error) error {
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	wrapped := *apiErr
	switch apiErr.Code {
	case driveMemberAddInvalidOperationCode:
		wrapped.Message = "Drive batch member add failed: one or more requested members may already be collaborators on this resource"
		wrapped.Hint = "For batch add, remove members that already have access (especially a bot/app being added again), then retry only the missing collaborators."
	case driveMemberAddInvalidParameterCode:
		wrapped.Message = "Drive batch member add failed: one or more requested members may be invalid for this resource or identity"
		wrapped.Hint = "Check whether each --member-id exists, belongs to the same tenant, and is visible to the current identity; remove invalid members and retry only the valid collaborators."
	default:
		return err
	}
	wrapped.Cause = err
	return &wrapped
}

func buildDriveMemberAddMemberBodies(spec driveMemberAddSpec) []map[string]interface{} {
	members := make([]map[string]interface{}, len(spec.MemberIDs))
	for i, mid := range spec.MemberIDs {
		members[i] = buildMemberBody(mid, spec.MemberType, spec.MemberKind, spec.Perm, spec.PermType)
	}
	return members
}

func buildDriveMemberAddBatchResult(spec driveMemberAddSpec, data map[string]interface{}) map[string]interface{} {
	rawMembers, _ := data["members"].([]interface{})

	// Build set of requested IDs for O(1) lookup.
	requestedSet := make(map[string]bool, len(spec.MemberIDs))
	for _, id := range spec.MemberIDs {
		requestedSet[id] = true
	}

	// First pass: build returned map and results array.
	// Matching is done by member_id, not by array index, so the server may
	// return members in any order without causing false partial_failure.
	results := make([]map[string]interface{}, 0, len(rawMembers))
	succeededIDs := make(map[string]bool, len(rawMembers))
	var mismatched []map[string]interface{}

	for _, raw := range rawMembers {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		rawMemberID := common.GetString(m, "member_id")

		out := driveMemberAddOutputWithOptions(spec, "", m, false)
		results = append(results, out)

		if rawMemberID != "" {
			if requestedSet[rawMemberID] {
				succeededIDs[rawMemberID] = true
			} else {
				mismatched = append(mismatched, map[string]interface{}{
					"returned": rawMemberID,
				})
			}
		}
	}

	// Second pass: find requested IDs missing from the response.
	missing := make([]string, 0)
	for _, memberID := range spec.MemberIDs {
		if !succeededIDs[memberID] {
			missing = append(missing, memberID)
		}
	}

	partial := len(results) != len(spec.MemberIDs) || len(missing) > 0 || len(mismatched) > 0
	result := map[string]interface{}{
		"resource_token":     spec.Token,
		"resource_type":      spec.ResourceType,
		"requested_count":    len(spec.MemberIDs),
		"succeeded_count":    len(succeededIDs),
		"partial":            partial,
		"members":            results,
		"missing_member_ids": missing,
	}
	if len(mismatched) > 0 {
		result["mismatched_member_ids"] = mismatched
	}
	return result
}

// driveMemberAddOutput flattens the server response into a stable envelope and
// backfills fields from spec when the server omits them.
func driveMemberAddOutput(spec driveMemberAddSpec, fallbackMemberID string, raw map[string]interface{}) map[string]interface{} {
	return driveMemberAddOutputWithOptions(spec, fallbackMemberID, raw, true)
}

func driveMemberAddOutputWithOptions(spec driveMemberAddSpec, fallbackMemberID string, raw map[string]interface{}, allowDefaultMemberID bool) map[string]interface{} {
	out := map[string]interface{}{
		"resource_token": spec.Token,
		"resource_type":  spec.ResourceType,
	}
	if raw != nil {
		for _, key := range []string{"member_id", "member_type", "perm", "type"} {
			if v, ok := raw[key]; ok {
				out[key] = v
			}
		}
		if spec.ResourceType == "wiki" {
			if v, ok := raw["perm_type"]; ok {
				out["perm_type"] = v
			}
		}
	}
	if common.GetString(out, "member_id") == "" {
		if fallbackMemberID == "" && allowDefaultMemberID && len(spec.MemberIDs) > 0 {
			fallbackMemberID = spec.MemberIDs[0]
		}
		if fallbackMemberID != "" {
			out["member_id"] = fallbackMemberID
		}
	}
	if common.GetString(out, "member_type") == "" {
		out["member_type"] = spec.MemberType
	}
	if common.GetString(out, "perm") == "" {
		out["perm"] = spec.Perm
	}
	if spec.PermType != "" && common.GetString(out, "perm_type") == "" {
		out["perm_type"] = spec.PermType
	}
	if bodyType := driveMemberAddBodyType(spec.MemberType, spec.MemberKind); bodyType != "" && common.GetString(out, "type") == "" {
		out["type"] = bodyType
	}
	if t := common.GetString(out, "type"); t != "" {
		out["member_kind"] = t
	}
	delete(out, "type")
	return out
}
