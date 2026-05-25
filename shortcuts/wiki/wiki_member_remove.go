// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// WikiMemberRemove wraps DELETE /open-apis/wiki/v2/spaces/{space_id}/members/{member_id}.
// Unlike most DELETEs, this API requires a body specifying member_type and
// member_role, since the path :member_id is ambiguous without both. The
// shortcut surfaces both as flags and flattens the returned member object.
var WikiMemberRemove = common.Shortcut{
	Service:     "wiki",
	Command:     "+member-remove",
	Description: "Remove a member from a wiki space",
	Risk:        "write",
	// The API also accepts wiki:wiki; we declare the narrowest valid scope so
	// tokens carrying only wiki:member:update aren't false-rejected by the
	// exact-string scope preflight (see +space-list for the full reasoning).
	Scopes:    []string{"wiki:member:update"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "space-id", Desc: "wiki space ID; use my_library for the personal document library (user only)", Required: true},
		{Name: "member-id", Desc: "member ID; interpretation is decided by --member-type", Required: true},
		{Name: "member-type", Desc: "ID type for --member-id (must match the original grant)", Required: true, Enum: wikiMemberTypes},
		{Name: "member-role", Desc: "role being revoked (must match the original grant)", Required: true, Enum: wikiMemberRoles},
	},
	Tips: []string{
		"--member-type and --member-role must match the original grant; revoking a non-existent (member_id, type, role) tuple is a no-op error from the API.",
		"To switch a member from admin to member or vice versa, remove the old role first, then call +member-add with the new one.",
		"--space-id my_library is a per-user alias and is only valid with --as user.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readWikiMemberRemoveSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readWikiMemberRemoveSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildWikiMemberRemoveDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readWikiMemberRemoveSpec(runtime)
		if err != nil {
			return err
		}

		spaceID, err := resolveWikiMemberSpaceID(runtime, spec.SpaceID)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Removing wiki space member %s (type=%s, role=%s) from space %s...\n",
			common.MaskToken(spec.MemberID), spec.MemberType, spec.MemberRole, common.MaskToken(spaceID))

		path := fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/members/%s",
			validate.EncodePathSegment(spaceID),
			validate.EncodePathSegment(spec.MemberID),
		)
		data, err := runtime.CallAPI("DELETE", path, nil, spec.RequestBody())
		if err != nil {
			return err
		}

		out := map[string]interface{}{"space_id": spaceID}
		for k, v := range wikiMemberRecord(common.GetMap(data, "member")) {
			out[k] = v
		}
		// Defensive default: if the API omits the member echo, or echoes empty
		// strings for any of the three identifying fields, fall back to the
		// caller's inputs per-field so scripts still see what was removed.
		if common.GetString(out, "member_id") == "" {
			out["member_id"] = spec.MemberID
		}
		if common.GetString(out, "member_type") == "" {
			out["member_type"] = spec.MemberType
		}
		if common.GetString(out, "member_role") == "" {
			out["member_role"] = spec.MemberRole
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Removed wiki space member %s\n", common.MaskToken(common.GetString(out, "member_id")))
		runtime.Out(out, nil)
		return nil
	},
}

// wikiMemberRemoveSpec is the normalized CLI input.
type wikiMemberRemoveSpec struct {
	SpaceID    string
	MemberID   string
	MemberType string
	MemberRole string
}

// RequestBody builds the JSON body the DELETE endpoint requires.
func (spec wikiMemberRemoveSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"member_type": spec.MemberType,
		"member_role": spec.MemberRole,
	}
}

func readWikiMemberRemoveSpec(runtime *common.RuntimeContext) (wikiMemberRemoveSpec, error) {
	spec := wikiMemberRemoveSpec{
		SpaceID:    strings.TrimSpace(runtime.Str("space-id")),
		MemberID:   strings.TrimSpace(runtime.Str("member-id")),
		MemberType: strings.ToLower(strings.TrimSpace(runtime.Str("member-type"))),
		MemberRole: strings.ToLower(strings.TrimSpace(runtime.Str("member-role"))),
	}
	if err := validateWikiMemberSpaceID(runtime, spec.SpaceID); err != nil {
		return wikiMemberRemoveSpec{}, err
	}
	if spec.MemberID == "" {
		return wikiMemberRemoveSpec{}, output.ErrValidation("--member-id is required and cannot be blank")
	}
	// Enum membership for --member-type / --member-role is enforced by the
	// framework's validateEnumFlags (runner.go) before Validate runs.
	return spec, nil
}

func buildWikiMemberRemoveDryRun(spec wikiMemberRemoveSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	if spec.SpaceID == wikiMyLibrarySpaceID {
		dry.Desc("2-step orchestration: resolve my_library -> remove wiki space member").
			GET("/open-apis/wiki/v2/spaces/my_library").
			Desc("[1] Resolve my_library space ID")
		dry.DELETE(fmt.Sprintf(
			"/open-apis/wiki/v2/spaces/%s/members/%s",
			"<resolved_space_id>",
			validate.EncodePathSegment(spec.MemberID),
		)).
			Desc("[2] Remove wiki space member").
			Body(spec.RequestBody())
		return dry
	}
	return dry.DELETE(fmt.Sprintf(
		"/open-apis/wiki/v2/spaces/%s/members/%s",
		validate.EncodePathSegment(spec.SpaceID),
		validate.EncodePathSegment(spec.MemberID),
	)).
		Body(spec.RequestBody())
}
