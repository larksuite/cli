// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// permApplyTypes is the authoritative list of type values the apply-permission
// endpoint accepts for its required `type` query parameter.
var permApplyTypes = []string{
	"doc", "sheet", "file", "wiki", "bitable", "docx",
	"mindnote", "slides",
}

// permApplyURLMarkers maps document URL path markers to the `type` value the
// apply-permission endpoint expects. Markers are disjoint strings (each begins
// with "/" and ends with "/"), so a simple substring scan disambiguates them.
var permApplyURLMarkers = []struct {
	Marker string
	Type   string
}{
	{"/wiki/", "wiki"},
	{"/docx/", "docx"},
	{"/sheets/", "sheet"},
	{"/base/", "bitable"},
	{"/bitable/", "bitable"},
	{"/file/", "file"},
	{"/mindnote/", "mindnote"},
	{"/slides/", "slides"},
	{"/doc/", "doc"},
}

// resolvePermApplyTarget extracts (token, type) from a user-supplied --token
// value that may be either a bare token or a full document URL, plus an
// optional explicit --type. Explicit --type wins over URL inference.
func resolvePermApplyTarget(raw, explicitType string) (token, docType string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", output.ErrValidation("--token is required")
	}

	if strings.Contains(raw, "://") {
		for _, m := range permApplyURLMarkers {
			if tok, ok := extractURLToken(raw, m.Marker); ok {
				token = tok
				if explicitType == "" {
					docType = m.Type
				}
				break
			}
		}
		if token == "" {
			return "", "", output.ErrValidation(
				"could not infer token from URL %q: supported paths are /docx/, /sheets/, /base/, /bitable/, /file/, /wiki/, /doc/, /mindnote/, /slides/. Pass a bare token with --type instead if the URL shape is unusual",
				raw,
			)
		}
	} else {
		token = raw
	}

	if explicitType != "" {
		docType = explicitType
	}
	if docType == "" {
		return "", "", output.ErrValidation(
			"--type is required when --token is a bare token; accepted values: %s",
			strings.Join(permApplyTypes, ", "),
		)
	}
	return token, docType, nil
}

// DriveApplyPermission applies to the document owner for view or edit access
// on behalf of the invoking user. Matches the open-apis endpoint
// /open-apis/drive/v1/permissions/:token/members/apply.
//
// The backend accepts only user_access_token for this endpoint, so the
// shortcut declares AuthTypes: ["user"] — bot identity is rejected up-front.
var DriveApplyPermission = common.Shortcut{
	Service:     "drive",
	Command:     "+apply-permission",
	Description: "Apply to the document owner for view or edit permission on a doc/sheet/file/wiki/bitable/docx/mindnote/slides",
	Risk:        "write",
	Scopes:      []string{"docs:permission.member:apply"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "token", Desc: "target token or document URL (docx/sheets/base/file/wiki/doc/mindnote/slides)", Required: true},
		{Name: "type", Desc: "target type; auto-inferred from URL when omitted", Enum: permApplyTypes},
		{Name: "perm", Desc: "permission to request", Required: true, Enum: []string{"view", "edit"}},
		{Name: "remark", Desc: "optional note shown on the request card sent to the owner"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, err := resolvePermApplyTarget(runtime.Str("token"), runtime.Str("type"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, docType, err := resolvePermApplyTarget(runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		body := buildPermApplyBody(runtime)
		return common.NewDryRunAPI().
			Desc("Apply to document owner for access").
			POST("/open-apis/drive/v1/permissions/:token/members/apply").
			Params(map[string]interface{}{"type": docType}).
			Body(body).
			Set("token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, docType, err := resolvePermApplyTarget(runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return err
		}
		body := buildPermApplyBody(runtime)

		fmt.Fprintf(runtime.IO().ErrOut, "Requesting %s access on %s %s...\n",
			runtime.Str("perm"), docType, common.MaskToken(token))

		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members/apply", validate.EncodePathSegment(token)),
			map[string]interface{}{"type": docType},
			body,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// buildPermApplyBody returns the request body with the caller-supplied perm
// and optional remark. remark is omitted entirely when empty so the server
// doesn't render an empty note on the request card.
func buildPermApplyBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{"perm": runtime.Str("perm")}
	if s := runtime.Str("remark"); s != "" {
		body["remark"] = s
	}
	return body
}
