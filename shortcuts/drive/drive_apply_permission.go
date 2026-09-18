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

type permApplyResourceKind struct {
	Type string
	Path string
}

// permApplyResourceKinds is the authoritative target contract for the
// apply-permission endpoint: accepted types and their URL root paths.
var permApplyResourceKinds = []permApplyResourceKind{
	{Type: "doc", Path: "/doc/"},
	{Type: "sheet", Path: "/sheets/"},
	{Type: "file", Path: "/file/"},
	{Type: "wiki", Path: "/wiki/"},
	{Type: "bitable", Path: "/base/"},
	{Type: "bitable", Path: "/bitable/"},
	{Type: "docx", Path: "/docx/"},
	{Type: "mindnote", Path: "/mindnote/"},
	{Type: "slides", Path: "/slides/"},
	{Type: "apps", Path: "/page/"},
}

var permApplyTypes = func() []string {
	types := make([]string, 0, len(permApplyResourceKinds))
	seen := make(map[string]struct{}, len(permApplyResourceKinds))
	for _, resourceKind := range permApplyResourceKinds {
		if _, ok := seen[resourceKind.Type]; ok {
			continue
		}
		seen[resourceKind.Type] = struct{}{}
		types = append(types, resourceKind.Type)
	}
	return types
}()

func permApplyTypeAllowed(docType string) bool {
	for _, allowedType := range permApplyTypes {
		if docType == allowedType {
			return true
		}
	}
	return false
}

// resolvePermApplyTarget extracts (token, type) from a user-supplied --token
// value that may be either a bare token or a full document URL, plus an
// optional explicit --type. A URL's path and explicit --type must agree.
func resolvePermApplyTarget(raw, explicitType string) (token, docType string, err error) {
	raw = strings.TrimSpace(raw)
	explicitType = strings.ToLower(strings.TrimSpace(explicitType))
	if raw == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token is required").WithParam("--token")
	}
	if explicitType != "" && !permApplyTypeAllowed(explicitType) {
		return "", "", errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"invalid --type %q: allowed values are %s",
			explicitType,
			strings.Join(permApplyTypes, ", "),
		).WithParam("--type")
	}

	if strings.Contains(raw, "://") {
		ref, ok := parsePermApplyResourceURL(raw)
		if !ok {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"could not infer token from URL %q: supported paths are /docx/, /sheets/, /base/, /bitable/, /file/, /wiki/, /doc/, /mindnote/, /slides/, /page/. Pass a bare token with --type instead if the URL shape is unusual",
				raw,
			).WithParam("--token")
		}
		token, docType = ref.Token, ref.Type
		if explicitType != "" && explicitType != docType {
			return "", "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				explicitType,
				docType,
			).WithParam("--type")
		}
	} else {
		token = raw
		docType = explicitType
	}

	if docType == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--type is required when --token is a bare token; accepted values: %s",
			strings.Join(permApplyTypes, ", "),
		).WithParam("--type")
	}
	if err := validatePermApplyToken(token); err != nil {
		return "", "", err
	}
	return token, docType, nil
}

func parsePermApplyResourceURL(rawURL string) (common.ResourceRef, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return common.ResourceRef{}, false
	}

	escapedPath := parsed.EscapedPath()
	for _, resourceKind := range permApplyResourceKinds {
		if !strings.HasPrefix(escapedPath, resourceKind.Path) {
			continue
		}
		escapedToken := strings.TrimSuffix(strings.TrimPrefix(escapedPath, resourceKind.Path), "/")
		if escapedToken == "" || strings.Contains(escapedToken, "/") {
			return common.ResourceRef{}, false
		}
		token, err := url.PathUnescape(escapedToken)
		if err != nil || token == "" {
			return common.ResourceRef{}, false
		}
		return common.ResourceRef{Type: resourceKind.Type, Token: token}, true
	}
	return common.ResourceRef{}, false
}

func validatePermApplyToken(token string) error {
	if err := validate.ResourceName(token, "--token"); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--token")
	}
	if token == "." || strings.Contains(token, "/") {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--token must be a non-dot single path segment",
		).WithParam("--token")
	}
	return nil
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
	Description: "Apply to the owner for view or edit permission on a Drive resource",
	Risk:        "write",
	Scopes:      []string{"docs:permission.member:apply"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "token", Desc: "target token or URL (docx/sheets/base/file/wiki/doc/mindnote/slides/page)", Required: true},
		{Name: "type", Desc: "target type; auto-inferred from URL when omitted", Enum: permApplyTypes},
		{Name: "perm", Desc: "permission to request", Required: true, Enum: []string{"view", "edit"}},
		{Name: "remark", Desc: "optional note shown on the request card sent to the owner"},
	},
	Tips: []string{
		"When --token is a URL, its path determines --type; a conflicting --type is rejected.",
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
			Desc("Apply to resource owner for access").
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

		data, err := runtime.CallAPITyped("POST",
			fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members/apply", validate.EncodePathSegment(token)),
			map[string]interface{}{"type": docType},
			body,
		)
		if err != nil {
			return decoratePermApplyError(err)
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

func decoratePermApplyError(err error) error {
	if err == nil {
		return nil
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	guidance := permApplyErrorGuidance(problem.Code)
	if guidance == "" {
		return err
	}
	if problem.Hint == "" {
		problem.Hint = guidance
	} else if !strings.Contains(problem.Hint, guidance) {
		problem.Hint += "; " + guidance
	}
	return err
}

func permApplyErrorGuidance(code int) string {
	switch code {
	case 1063006:
		return "permission-apply quota reached: each user may request access on the same document at most 5 times per day; wait for the daily quota to reset before retrying"
	case 1063007:
		return "this document does not accept a permission-apply request; verify the target and requested permission, or contact the owner directly"
	default:
		return ""
	}
}
