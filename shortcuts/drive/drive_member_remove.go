// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveMemberRemoveIDTypes = []string{
	"email", "openid", "openchat", "opendepartmentid",
	"userid", "unionid", "groupid", "appid", "wikispaceid",
}

// DriveMemberRemove removes one collaborator/member permission from a Drive resource.
var DriveMemberRemove = common.Shortcut{
	Service:     "drive",
	Command:     "+member-remove",
	Description: "Remove one collaborator/member permission from a Drive resource",
	Risk:        "high-risk-write",
	Scopes:      []string{"docs:permission.member:delete"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "target token or document URL; type is auto-inferred from URL path when omitted", Required: true},
		{Name: "type", Desc: "target resource type; required when --token is a bare token"},
		{Name: "member-id", Desc: "single collaborator ID to remove; comma-separated values are rejected", Required: true},
		{Name: "member-type", Desc: "ID type for --member-id; supported: email|openid|openchat|opendepartmentid|userid|unionid|groupid|appid|wikispaceid", Required: true},
		{Name: "member-kind", Desc: "request body type when --member-type=wikispaceid; one of wiki_space_member|wiki_space_viewer|wiki_space_editor"},
		{Name: "perm-type", Desc: "wiki permission scope; defaults to container; rejected for non-wiki types and wiki-space members"},
	},
	Tips: []string{
		"This command removes exactly one collaborator; run separate commands for multiple members.",
		"Resource type is auto-inferred from URL paths; pass --type when --token is a bare token.",
		"When --member-type=wikispaceid, pass --member-kind wiki_space_member, wiki_space_viewer, or wiki_space_editor.",
		"For ordinary wiki collaborators, --perm-type defaults to container; use single_page to remove only the current-page permission.",
		"Department collaborator removal (--member-type=opendepartmentid) requires --as user; bot identity is not supported.",
		"A successful response confirms that the removal request completed; it does not prove the permission previously existed.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveMemberRemoveSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveMemberRemoveSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveMemberRemoveDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveMemberRemoveSpec(runtime)
		if err != nil {
			return err
		}
		return executeDriveMemberRemove(runtime, spec)
	},
}

type driveMemberRemoveSpec struct {
	Token        string
	ResourceType string
	MemberID     string
	MemberType   string
	MemberKind   string
	PermType     string
}

// APIQueryParams returns the query parameters sent with the member-remove DELETE request.
func (spec driveMemberRemoveSpec) APIQueryParams() map[string]interface{} {
	return map[string]interface{}{
		"type":        spec.ResourceType,
		"member_type": spec.MemberType,
	}
}

// Body builds the request body, carrying the wiki member kind and permission scope only when set.
func (spec driveMemberRemoveSpec) Body() map[string]interface{} {
	body := make(map[string]interface{}, 2)
	if memberKind := driveMemberAddBodyType(spec.MemberType, spec.MemberKind); memberKind != "" {
		body["type"] = memberKind
	}
	if spec.PermType != "" {
		body["perm_type"] = spec.PermType
	}
	return body
}

// readDriveMemberRemoveSpec reads and validates the command flags into a driveMemberRemoveSpec,
// enforcing single-member, member-type, wiki, and identity constraints before any request is issued.
// rejectDriveMemberRemoveDotSegment rejects a value that is an RFC 3986 dot
// segment ("." or "..") in any "/"-split part. validate.ResourceName only stops
// "..", but a lone "." (or an encoded "%2e" that PathUnescape decodes to ".")
// survives EncodePathSegment unchanged and can be normalized away by an HTTP
// stack or proxy, redirecting this high-risk DELETE to a different route. This
// is a local, remove-only guard so the shared validate.ResourceName behavior is
// left untouched for its many other callers.
func rejectDriveMemberRemoveDotSegment(value, flagName string) error {
	for _, seg := range strings.Split(value, "/") {
		if seg == "." || seg == ".." {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"%s must not contain '.' or '..' path segments",
				flagName,
			).WithParam(flagName)
		}
	}
	return nil
}

func readDriveMemberRemoveSpec(runtime *common.RuntimeContext) (driveMemberRemoveSpec, error) {
	token, resourceType, err := resolveDriveMemberRemoveTarget(runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveMemberRemoveSpec{}, err
	}
	// The resolver already rejects separators, but not "." or "..". Validate the
	// resolved (decoded) token so a bare ".", "..", or an encoded "%2e"/"%2e%2e"
	// in a URL path cannot reach the DELETE path as a dot segment.
	if err := validate.ResourceName(token, "--token"); err != nil {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument, "%s", err.Error(),
		).WithParam("--token")
	}
	if err := rejectDriveMemberRemoveDotSegment(token, "--token"); err != nil {
		return driveMemberRemoveSpec{}, err
	}

	memberID := strings.TrimSpace(runtime.Str("member-id"))
	if memberID == "" {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--member-id is required and cannot be blank",
		).WithParam("--member-id")
	}
	if strings.Contains(memberID, "/") {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--member-id must be a single collaborator ID and cannot contain '/'",
		).WithParam("--member-id")
	}
	if strings.Contains(memberID, ",") {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--member-id accepts exactly one collaborator ID; run one +member-remove command per member",
		).WithParam("--member-id")
	}
	// Reject path-traversal and other unsafe patterns (e.g. "..", "%2e%2e" decoded
	// to "..") before this value is interpolated into the DELETE path. The
	// single-segment checks above stop separators, but not "." or "..", which
	// EncodePathSegment leaves intact and an HTTP stack/proxy could normalize into
	// a different route. This is a high-risk delete, so validate defensively.
	if err := validate.ResourceName(memberID, "--member-id"); err != nil {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument, "%s", err.Error(),
		).WithParam("--member-id")
	}
	if err := rejectDriveMemberRemoveDotSegment(memberID, "--member-id"); err != nil {
		return driveMemberRemoveSpec{}, err
	}

	memberType, err := resolveDriveMemberRemoveMemberType(memberID, runtime.Str("member-type"))
	if err != nil {
		return driveMemberRemoveSpec{}, err
	}
	if memberType == "wikispaceid" && resourceType != "wiki" {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--member-type=wikispaceid only applies when resource type is wiki; got %q",
			resourceType,
		).WithParam("--member-type")
	}
	memberKind, err := resolveDriveMemberAddMemberKind(memberType, runtime.Str("member-kind"))
	if err != nil {
		return driveMemberRemoveSpec{}, err
	}

	permType, err := normalizeDriveMemberAddEnumValue(runtime.Str("perm-type"), driveMemberAddPermTypes, "--perm-type")
	if err != nil {
		return driveMemberRemoveSpec{}, err
	}
	if resourceType == "wiki" && memberType == "wikispaceid" {
		if runtime.Changed("perm-type") {
			return driveMemberRemoveSpec{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--perm-type is not supported when --member-type=wikispaceid; use --member-kind wiki_space_member|wiki_space_viewer|wiki_space_editor",
			).WithParam("--perm-type")
		}
		permType = ""
	} else if resourceType == "wiki" && permType == "" {
		permType = driveMemberAddDefaultPermType(resourceType)
	} else if resourceType != "wiki" && runtime.Changed("perm-type") {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--perm-type only applies when resource type is wiki; got %q",
			resourceType,
		).WithParam("--perm-type")
	} else if resourceType != "wiki" {
		permType = ""
	}

	spec := driveMemberRemoveSpec{
		Token:        token,
		ResourceType: resourceType,
		MemberID:     memberID,
		MemberType:   memberType,
		MemberKind:   memberKind,
		PermType:     permType,
	}
	if runtime.As().IsBot() && spec.MemberType == "opendepartmentid" {
		return driveMemberRemoveSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--member-type=opendepartmentid requires --as user; bot identity does not support removing department collaborators",
		).WithParam("--member-type")
	}
	return spec, nil
}

// resolveDriveMemberRemoveMemberType normalizes and requires --member-type, rejecting a value
// that conflicts with the prefix implied by memberID (except tenant-defined userid).
func resolveDriveMemberRemoveMemberType(memberID, explicit string) (string, error) {
	memberType, err := normalizeDriveMemberAddEnumValue(explicit, driveMemberRemoveIDTypes, "--member-type")
	if err != nil {
		return "", err
	}
	if memberType == "" {
		return "", errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--member-type is required; accepted values: %s",
			strings.Join(driveMemberRemoveIDTypes, ", "),
		).WithParam("--member-type")
	}

	// User IDs are tenant-defined and may resemble another supported ID format.
	if memberType != "userid" {
		if expected := inferMemberTypeFromID(memberID); expected != "" && expected != memberType {
			return "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"member-id %q prefix implies --member-type %s, but --member-type %s was provided; fix the ID or use the matching member type",
				memberID,
				expected,
				memberType,
			).WithParam("--member-id")
		}
	}
	return memberType, nil
}

// buildDriveMemberRemoveDryRun renders the spec as a dry-run description of the DELETE request.
func buildDriveMemberRemoveDryRun(spec driveMemberRemoveSpec) *common.DryRunAPI {
	return common.NewDryRunAPI().
		Desc("Remove Drive collaborator/member permission").
		DELETE(driveMemberRemovePath(spec)).
		Params(spec.APIQueryParams()).
		Body(spec.Body())
}

// driveMemberRemovePath builds the permissions member DELETE path with each segment escaped.
func driveMemberRemovePath(spec driveMemberRemoveSpec) string {
	return fmt.Sprintf(
		"/open-apis/drive/v1/permissions/%s/members/%s",
		validate.EncodePathSegment(spec.Token),
		validate.EncodePathSegment(spec.MemberID),
	)
}

// executeDriveMemberRemove issues the typed DELETE request and emits the structured removal result.
func executeDriveMemberRemove(runtime *common.RuntimeContext, spec driveMemberRemoveSpec) error {
	fmt.Fprintf(
		runtime.IO().ErrOut,
		"Removing Drive member %s (type=%s) from %s %s...\n",
		common.MaskToken(spec.MemberID),
		spec.MemberType,
		spec.ResourceType,
		common.MaskToken(spec.Token),
	)

	if _, err := runtime.CallAPITyped(
		"DELETE",
		driveMemberRemovePath(spec),
		spec.APIQueryParams(),
		spec.Body(),
	); err != nil {
		return err
	}

	out := driveMemberRemoveOutput(spec)

	fmt.Fprintf(runtime.IO().ErrOut, "Removed Drive member %s\n", common.MaskToken(spec.MemberID))
	runtime.Out(out, nil)
	return nil
}

// driveMemberRemoveOutput assembles the structured output map, including optional member kind and perm type.
func driveMemberRemoveOutput(spec driveMemberRemoveSpec) map[string]interface{} {
	out := map[string]interface{}{
		"removed":        true,
		"resource_token": spec.Token,
		"resource_type":  spec.ResourceType,
		"member_id":      spec.MemberID,
		"member_type":    spec.MemberType,
	}
	if memberKind := driveMemberAddBodyType(spec.MemberType, spec.MemberKind); memberKind != "" {
		out["member_kind"] = memberKind
	}
	if spec.PermType != "" {
		out["perm_type"] = spec.PermType
	}
	return out
}

// resolveDriveMemberRemoveTarget resolves (token, type) from a --token that may
// be a bare token or a resource URL, plus an optional explicit --type. Because
// +member-remove is a high-risk delete, it parses URLs strictly: the escaped
// path must be a supported prefix followed by exactly one segment, and a
// --type that conflicts with the URL path type is rejected. This deliberately
// does not reuse the more permissive +member-add resolver, so an ambiguous or
// encoded URL can never delete a collaborator from an unintended resource.
func resolveDriveMemberRemoveTarget(raw, explicitType string) (token, resourceType string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token is required").WithParam("--token")
	}
	explicitType = strings.ToLower(strings.TrimSpace(explicitType))

	if explicitType != "" && !isSupportedDriveMemberAddResourceType(explicitType) {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--type must be one of: %s", strings.Join(driveMemberAddResourceTypes, ", "),
		).WithParam("--type")
	}

	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Hostname() == "" {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token URL is malformed: %q", raw).WithParam("--token")
		}
		urlToken, urlType, ok := parseDriveMemberRemoveResourceURLPath(parsed.EscapedPath())
		if !ok {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unsupported URL path %q: expected one of %s followed by a single token segment",
				parsed.EscapedPath(), strings.Join(driveMemberAddSupportedURLPaths(), ", "),
			).WithParam("--token")
		}
		if explicitType != "" && explicitType != urlType {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				explicitType, urlType,
			).WithParam("--type")
		}
		return urlToken, urlType, nil
	}

	if explicitType == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--type is required when --token is a bare token; accepted values: %s",
			strings.Join(driveMemberAddResourceTypes, ", "),
		).WithParam("--type")
	}
	if strings.Contains(raw, "/") {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--token must resolve to a single resource token and cannot contain '/'",
		).WithParam("--token")
	}
	return raw, explicitType, nil
}

// parseDriveMemberRemoveResourceURLPath resolves (token, type) from a URL path
// with strict exact-segment matching: the escaped path must be a supported
// prefix followed by exactly one path segment. Extra segments or encoded
// separators (%2F) are rejected rather than truncated, so a URL can never
// silently address a different resource than the one it spells out.
func parseDriveMemberRemoveResourceURLPath(escapedPath string) (token, resourceType string, ok bool) {
	for _, mapping := range driveMemberAddURLPathToType {
		if !strings.HasPrefix(escapedPath, mapping.Prefix) {
			continue
		}
		escapedToken := strings.TrimSuffix(strings.TrimPrefix(escapedPath, mapping.Prefix), "/")
		if escapedToken == "" || strings.Contains(escapedToken, "/") {
			return "", "", false
		}
		decoded, decodeErr := url.PathUnescape(escapedToken)
		if decodeErr != nil || decoded == "" || strings.Contains(decoded, "/") {
			return "", "", false
		}
		return decoded, mapping.Type, true
	}
	return "", "", false
}
