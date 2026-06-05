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

// WikiMemberAdd wraps POST /open-apis/wiki/v2/spaces/{space_id}/members. The
// shortcut adds flag ergonomics over the raw API: explicit --member-type and
// --member-role enum hints, optional --need-notification, my_library
// resolution, and a flattened single-member output envelope.
var WikiMemberAdd = common.Shortcut{
	Service:     "wiki",
	Command:     "+member-add",
	Description: "Add a member to a wiki space",
	Risk:        "write",
	// The API also accepts wiki:wiki, but the framework's preflight does
	// exact-string scope matching (see +space-list), so declare the narrowest
	// scope so tokens that only carry wiki:member:create aren't false-rejected.
	Scopes:    []string{"wiki:member:create"},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "space-id", Desc: "wiki space ID; use my_library for the personal document library (user only)", Required: true},
		{Name: "member-id", Desc: "member ID; interpretation is decided by --member-type", Required: true},
		{Name: "member-type", Desc: "ID type for --member-id", Required: true, Enum: wikiMemberTypes},
		{Name: "member-role", Desc: "role granted within the space", Required: true, Enum: wikiMemberRoles},
		{Name: "need-notification", Type: "bool", Desc: "send an in-app notification to the new member after the grant"},
	},
	Tips: []string{
		"Use --member-type=email with the user's mailbox if you do not know their open_id.",
		"--member-role=admin grants full space administration; pick --member-role=member for collaborator access.",
		"--space-id my_library is a per-user alias and is only valid with --as user.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readWikiMemberAddSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readWikiMemberAddSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildWikiMemberAddDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readWikiMemberAddSpec(runtime)
		if err != nil {
			return err
		}

		spaceID, err := resolveWikiMemberSpaceID(runtime, spec.SpaceID)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Adding wiki space member %s (type=%s, role=%s) to space %s...\n",
			common.MaskToken(spec.MemberID), spec.MemberType, spec.MemberRole, common.MaskToken(spaceID))

		path := fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/members", validate.EncodePathSegment(spaceID))
		data, err := runtime.CallAPI("POST", path, spec.QueryParams(), spec.RequestBody())
		if err != nil {
			return err
		}

		out := wikiMemberAddOutput(spaceID, common.GetMap(data, "member"))
		// Defensive default: mirror +member-remove and fall back to the caller's
		// inputs per-field when the API echoes empty strings or omits member
		// fields, so scripts always see what was added.
		if common.GetString(out, "member_id") == "" {
			out["member_id"] = spec.MemberID
		}
		if common.GetString(out, "member_type") == "" {
			out["member_type"] = spec.MemberType
		}
		if common.GetString(out, "member_role") == "" {
			out["member_role"] = spec.MemberRole
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Added wiki space member %s\n", common.MaskToken(common.GetString(out, "member_id")))
		runtime.Out(out, nil)
		return nil
	},
}

// wikiMemberAddSpec is the normalized CLI input.
type wikiMemberAddSpec struct {
	SpaceID          string
	MemberID         string
	MemberType       string
	MemberRole       string
	NeedNotification bool
	NotificationSet  bool
}

// RequestBody builds the JSON body for POST /spaces/{id}/members.
func (spec wikiMemberAddSpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"member_id":   spec.MemberID,
		"member_type": spec.MemberType,
		"member_role": spec.MemberRole,
	}
}

// QueryParams returns nil unless the caller explicitly set --need-notification,
// so the request stays clean when the flag is omitted instead of always
// forcing need_notification=false.
func (spec wikiMemberAddSpec) QueryParams() map[string]interface{} {
	if !spec.NotificationSet {
		return nil
	}
	return map[string]interface{}{"need_notification": spec.NeedNotification}
}

func readWikiMemberAddSpec(runtime *common.RuntimeContext) (wikiMemberAddSpec, error) {
	spec := wikiMemberAddSpec{
		SpaceID:          strings.TrimSpace(runtime.Str("space-id")),
		MemberID:         strings.TrimSpace(runtime.Str("member-id")),
		MemberType:       strings.ToLower(strings.TrimSpace(runtime.Str("member-type"))),
		MemberRole:       strings.ToLower(strings.TrimSpace(runtime.Str("member-role"))),
		NeedNotification: runtime.Bool("need-notification"),
		NotificationSet:  runtime.Cmd.Flags().Changed("need-notification"),
	}
	if err := validateWikiMemberSpaceID(runtime, spec.SpaceID); err != nil {
		return wikiMemberAddSpec{}, err
	}
	if spec.MemberID == "" {
		return wikiMemberAddSpec{}, output.ErrValidation("--member-id is required and cannot be blank")
	}
	// The space-member API rejects opendepartmentid grants under a
	// tenant_access_token; surface that as a CLI validation error so callers do
	// not waste a network round-trip on a server-side 403. The escape hatch is
	// --as user, which is the only identity the API accepts for departments.
	if runtime.As().IsBot() && spec.MemberType == "opendepartmentid" {
		return wikiMemberAddSpec{}, output.ErrValidation(
			"--as bot does not support --member-type opendepartmentid; rerun with --as user",
		)
	}
	// --member-type / --member-role enum membership is enforced by the
	// framework's validateEnumFlags (runner.go) before Validate runs, so no
	// extra membership check is needed here.
	return spec, nil
}

func buildWikiMemberAddDryRun(spec wikiMemberAddSpec) *common.DryRunAPI {
	dry := common.NewDryRunAPI()
	if spec.SpaceID == wikiMyLibrarySpaceID {
		dry.Desc("2-step orchestration: resolve my_library -> add wiki space member").
			GET("/open-apis/wiki/v2/spaces/my_library").
			Desc("[1] Resolve my_library space ID")
		dry.POST(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/members", "<resolved_space_id>")).
			Desc("[2] Add wiki space member").
			Params(spec.QueryParams()).
			Body(spec.RequestBody())
		return dry
	}
	return dry.POST(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/members", validate.EncodePathSegment(spec.SpaceID))).
		Params(spec.QueryParams()).
		Body(spec.RequestBody())
}

// wikiMemberAddOutput flattens data.member onto a top-level envelope so
// scripts can read member fields without traversing the nested response.
func wikiMemberAddOutput(spaceID string, raw map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{"space_id": spaceID}
	for k, v := range wikiMemberRecord(raw) {
		out[k] = v
	}
	return out
}
