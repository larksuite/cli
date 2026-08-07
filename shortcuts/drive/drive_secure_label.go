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

const (
	secureLabelReadScope   = "docs:secure_label:readonly"
	secureLabelUpdateScope = "docs:secure_label:write_only"
)

type secureLabelOperation string

const (
	secureLabelOperationList   secureLabelOperation = "list"
	secureLabelOperationUpdate secureLabelOperation = "update"
)

type secureLabelResourceKind struct {
	Type string
	Path string
}

// secureLabelResourceKinds is intentionally independent from apply-permission:
// the two endpoints accept different resource type contracts.
var secureLabelResourceKinds = []secureLabelResourceKind{
	{Type: "doc", Path: "/doc/"},
	{Type: "sheet", Path: "/sheets/"},
	{Type: "file", Path: "/file/"},
	{Type: "wiki", Path: "/wiki/"},
	{Type: "bitable", Path: "/base/"},
	{Type: "bitable", Path: "/bitable/"},
	{Type: "docx", Path: "/docx/"},
	{Type: "mindnote", Path: "/mindnote/"},
	{Type: "slides", Path: "/slides/"},
}

var secureLabelTypes = func() []string {
	types := make([]string, 0, len(secureLabelResourceKinds))
	seen := make(map[string]struct{}, len(secureLabelResourceKinds))
	for _, resourceKind := range secureLabelResourceKinds {
		if _, ok := seen[resourceKind.Type]; ok {
			continue
		}
		seen[resourceKind.Type] = struct{}{}
		types = append(types, resourceKind.Type)
	}
	return types
}()

func secureLabelTypeAllowed(docType string) bool {
	for _, allowedType := range secureLabelTypes {
		if docType == allowedType {
			return true
		}
	}
	return false
}

// DriveSecureLabelList lists secure labels available to the current user.
var DriveSecureLabelList = common.Shortcut{
	Service:     "drive",
	Command:     "+secure-label-list",
	Description: "List secure labels available to the current user",
	Risk:        common.RiskRead,
	Scopes:      []string{secureLabelReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Tips: []string{
		"Use the `id` field from this command as --label-id for +secure-label-update; do not use the display name.",
	},
	Flags: []common.Flag{
		{Name: "page-size", Type: "int", Default: "10", Desc: "page size, 1-10"},
		{Name: "page-token", Desc: "pagination token from previous response"},
		{Name: "lang", Desc: "label language", Enum: []string{"zh", "en", "ja"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pageSize := runtime.Int("page-size")
		if pageSize < 1 || pageSize > 10 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be between 1 and 10").WithParam("--page-size")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("List secure labels available to the current user").
			GET("/open-apis/drive/v2/my_secure_labels").
			Params(buildSecureLabelListParams(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped("GET",
			"/open-apis/drive/v2/my_secure_labels",
			buildSecureLabelListParams(runtime),
			nil,
		)
		if err != nil {
			return decorateSecureLabelError(err, secureLabelOperationList)
		}
		runtime.OutFormat(data, nil, nil)
		return nil
	},
}

// DriveSecureLabelUpdate updates the secure label on a Drive file/document.
var DriveSecureLabelUpdate = common.Shortcut{
	Service:     "drive",
	Command:     "+secure-label-update",
	Description: "Update the secure label on a Drive file or document",
	Risk:        common.RiskWrite,
	Scopes:      []string{secureLabelUpdateScope},
	AuthTypes:   []string{"user"},
	Tips: []string{
		"Pass the numeric label id returned by +secure-label-list; display names like Public(D) are rejected.",
		"When --token is a URL, its path determines --type; a conflicting --type is rejected.",
		"Downgrading a secure label may require approval; retrying the same request will not bypass approval.",
		"When updating many files, serialize requests and back off on rate_limit errors.",
	},
	Flags: []common.Flag{
		{Name: "token", Desc: "target file token or document URL (docx/sheets/base/file/wiki/doc/mindnote/slides)", Required: true},
		{Name: "type", Desc: "target type; auto-inferred from URL when omitted", Enum: secureLabelTypes},
		{Name: "label-id", Desc: "secure label ID to set", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, _, err := resolveSecureLabelTarget(runtime.Str("token"), runtime.Str("type")); err != nil {
			return err
		}
		_, err := normalizeSecureLabelID(runtime.Str("label-id"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, docType, err := resolveSecureLabelTarget(runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		labelID, err := normalizeSecureLabelID(runtime.Str("label-id"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			Desc("Update Drive secure label").
			PATCH("/open-apis/drive/v2/files/:file_token/secure_label").
			Params(map[string]interface{}{"type": docType}).
			Body(map[string]interface{}{"id": labelID}).
			Set("file_token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, docType, err := resolveSecureLabelTarget(runtime.Str("token"), runtime.Str("type"))
		if err != nil {
			return err
		}
		labelID, err := normalizeSecureLabelID(runtime.Str("label-id"))
		if err != nil {
			return err
		}
		body := map[string]interface{}{"id": labelID}
		data, err := runtime.CallAPITyped("PATCH",
			fmt.Sprintf("/open-apis/drive/v2/files/%s/secure_label", validate.EncodePathSegment(token)),
			map[string]interface{}{"type": docType},
			body,
		)
		if err != nil {
			return decorateSecureLabelError(err, secureLabelOperationUpdate)
		}
		runtime.Out(data, nil)
		return nil
	},
}

func buildSecureLabelListParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{"page_size": runtime.Int("page-size")}
	if pageToken := runtime.Str("page-token"); pageToken != "" {
		params["page_token"] = pageToken
	}
	if lang := runtime.Str("lang"); lang != "" {
		params["lang"] = lang
	}
	return params
}

// resolveSecureLabelTarget owns secure-label URL inference and type errors so
// changes to another endpoint cannot widen this command's accepted resources.
func resolveSecureLabelTarget(raw, explicitType string) (token, docType string, err error) {
	raw = strings.TrimSpace(raw)
	explicitType = strings.ToLower(strings.TrimSpace(explicitType))
	if raw == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token is required").WithParam("--token")
	}
	if explicitType != "" && !secureLabelTypeAllowed(explicitType) {
		return "", "", errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"invalid --type %q: allowed values are %s",
			explicitType,
			strings.Join(secureLabelTypes, ", "),
		).WithParam("--type")
	}

	if strings.Contains(raw, "://") {
		ref, ok := parseSecureLabelResourceURL(raw)
		if !ok {
			return "", "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"could not infer token from URL %q: supported paths are /docx/, /sheets/, /base/, /bitable/, /file/, /wiki/, /doc/, /mindnote/, /slides/. Pass a bare token with --type instead if the URL shape is unusual",
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
		return "", "", errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--type is required when --token is a bare token; accepted values: %s",
			strings.Join(secureLabelTypes, ", "),
		).WithParam("--type")
	}
	if err := validateSecureLabelToken(token); err != nil {
		return "", "", err
	}
	return token, docType, nil
}

func parseSecureLabelResourceURL(rawURL string) (common.ResourceRef, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return common.ResourceRef{}, false
	}

	escapedPath := parsed.EscapedPath()
	for _, resourceKind := range secureLabelResourceKinds {
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

func validateSecureLabelToken(token string) error {
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

// normalizeSecureLabelID trims a label id and rejects display names before the
// request reaches Drive, where they otherwise surface as opaque JSON errors.
func normalizeSecureLabelID(raw string) (string, error) {
	labelID := strings.TrimSpace(raw)
	if labelID == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--label-id is required").
			WithParam("--label-id")
	}
	for _, r := range labelID {
		if r < '0' || r > '9' {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--label-id must be a numeric secure label ID, not a display name: %q", raw).
				WithParam("--label-id").
				WithHint("run `lark-cli drive +secure-label-list` and pass the numeric `id` value; do not pass label names like `Public(D)`")
		}
	}
	return labelID, nil
}

// decorateSecureLabelError appends command-aware recovery guidance while
// preserving upstream/classifier hints already attached to the typed error.
func decorateSecureLabelError(err error, operation secureLabelOperation) error {
	if err == nil {
		return nil
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	guidance := secureLabelErrorGuidance(p.Code, operation)
	if guidance == "" {
		return err
	}
	if p.Hint == "" {
		p.Hint = guidance
	} else if !strings.Contains(p.Hint, guidance) {
		p.Hint = p.Hint + "; " + guidance
	}
	return err
}

// secureLabelErrorGuidance returns recovery guidance for secure-label API
// failures whose generic code-level classification needs command context.
func secureLabelErrorGuidance(code int, operation secureLabelOperation) string {
	switch code {
	case 99991400:
		if operation == secureLabelOperationUpdate {
			return "secure label updates are rate limited; retry later with exponential backoff and serialize bulk updates"
		}
		return "secure label listing is rate limited; retry later with exponential backoff"
	case 1063013:
		if operation == secureLabelOperationUpdate {
			return "secure label downgrade requires approval; request approval or choose a non-downgrade label before retrying"
		}
	case 1063002:
		if operation == secureLabelOperationUpdate {
			return "the current user lacks permission to update this file's secure label; use a user with file and security-label permission"
		}
		return "the current user lacks permission to list secure labels; use a user with security-label read permission"
	case 1063001, 99992402, 9499:
		if operation == secureLabelOperationUpdate {
			return "check --token/--type and pass a secure label ID from `lark-cli drive +secure-label-list`, not the display name"
		}
		return "check secure label list parameters such as --page-size, --page-token, and --lang"
	}
	return ""
}
