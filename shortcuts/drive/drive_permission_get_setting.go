// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type drivePermissionGetSettingSpec struct {
	Token string
	Type  string
}

type drivePermissionGetSettingResourceKind struct {
	Type          string
	CanonicalPath string
	PathAliases   []string
}

var drivePermissionGetSettingResourceKinds = []drivePermissionGetSettingResourceKind{
	{Type: "doc", CanonicalPath: "/doc/"},
	{Type: "sheet", CanonicalPath: "/sheets/"},
	{Type: "file", CanonicalPath: "/file/"},
	{Type: "wiki", CanonicalPath: "/wiki/"},
	{Type: "bitable", CanonicalPath: "/base/", PathAliases: []string{"/bitable/"}},
	{Type: "docx", CanonicalPath: "/docx/"},
	{Type: "mindnote", CanonicalPath: "/mindnote/", PathAliases: []string{"/mindnotes/"}},
	{Type: "minutes", CanonicalPath: "/minutes/"},
	{Type: "slides", CanonicalPath: "/slides/"},
	{Type: "folder", CanonicalPath: "/drive/folder/"},
	{Type: "apps", CanonicalPath: "/page/"},
}

var drivePermissionGetSettingTypes = func() []string {
	types := make([]string, 0, len(drivePermissionGetSettingResourceKinds))
	for _, resourceKind := range drivePermissionGetSettingResourceKinds {
		types = append(types, resourceKind.Type)
	}
	return types
}()

func findDrivePermissionGetSettingResourceKind(docType string) (drivePermissionGetSettingResourceKind, bool) {
	for _, resourceKind := range drivePermissionGetSettingResourceKinds {
		if docType == resourceKind.Type {
			return resourceKind, true
		}
	}
	return drivePermissionGetSettingResourceKind{}, false
}

func parseDrivePermissionGetSettingResourcePath(path, prefix, docType string) (common.ResourceRef, bool) {
	if !strings.HasPrefix(path, prefix) {
		return common.ResourceRef{}, false
	}
	escapedToken := strings.TrimSuffix(path[len(prefix):], "/")
	if escapedToken == "" || strings.Contains(escapedToken, "/") {
		return common.ResourceRef{}, false
	}
	token, err := url.PathUnescape(escapedToken)
	if err != nil || token == "" {
		return common.ResourceRef{}, false
	}
	return common.ResourceRef{Type: docType, Token: token}, true
}

func parseDrivePermissionGetSettingResourceKindPath(path string, resourceKind drivePermissionGetSettingResourceKind) (common.ResourceRef, bool) {
	if ref, ok := parseDrivePermissionGetSettingResourcePath(path, resourceKind.CanonicalPath, resourceKind.Type); ok {
		return ref, true
	}
	for _, alias := range resourceKind.PathAliases {
		if ref, ok := parseDrivePermissionGetSettingResourcePath(path, alias, resourceKind.Type); ok {
			return ref, true
		}
	}
	return common.ResourceRef{}, false
}

func readDrivePermissionGetSettingSpec(runtime *common.RuntimeContext) (drivePermissionGetSettingSpec, error) {
	rawToken := strings.TrimSpace(runtime.Str("token"))
	explicitType := strings.ToLower(strings.TrimSpace(runtime.Str("type")))

	if rawToken == "" {
		return drivePermissionGetSettingSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--token is required",
		).WithParam("--token")
	}

	if explicitType != "" && !drivePermissionGetSettingTypeAllowed(explicitType) {
		return drivePermissionGetSettingSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"invalid --type %q: allowed values are %s",
			explicitType,
			strings.Join(drivePermissionGetSettingTypes, ", "),
		).WithParam("--type")
	}

	if strings.Contains(rawToken, "://") {
		ref, ok := parseDrivePermissionGetSettingResourceURL(rawToken)
		if !ok {
			return drivePermissionGetSettingSpec{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported --token URL %q: pass a recognized Lark Drive resource URL or a bare token with --type",
				rawToken,
			).WithParam("--token")
		}
		if explicitType != "" && explicitType != ref.Type {
			return drivePermissionGetSettingSpec{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				explicitType,
				ref.Type,
			).WithParam("--type")
		}
		if err := validateDrivePermissionGetSettingToken(ref.Token); err != nil {
			return drivePermissionGetSettingSpec{}, err
		}
		return drivePermissionGetSettingSpec{Token: ref.Token, Type: ref.Type}, nil
	}

	if explicitType == "" {
		return drivePermissionGetSettingSpec{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--type is required when --token is a bare token (allowed: %s)",
			strings.Join(drivePermissionGetSettingTypes, ", "),
		).WithParam("--type")
	}

	if err := validateDrivePermissionGetSettingToken(rawToken); err != nil {
		return drivePermissionGetSettingSpec{}, err
	}
	return drivePermissionGetSettingSpec{Token: rawToken, Type: explicitType}, nil
}

func parseDrivePermissionGetSettingResourceURL(rawURL string) (common.ResourceRef, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return common.ResourceRef{}, false
	}

	for _, resourceKind := range drivePermissionGetSettingResourceKinds {
		if ref, ok := parseDrivePermissionGetSettingResourceKindPath(parsed.EscapedPath(), resourceKind); ok {
			return ref, true
		}
	}

	return common.ResourceRef{}, false
}

func drivePermissionGetSettingTypeAllowed(docType string) bool {
	_, ok := findDrivePermissionGetSettingResourceKind(docType)
	return ok
}

func (s drivePermissionGetSettingSpec) url(runtime *common.RuntimeContext) string {
	resourceKind, ok := findDrivePermissionGetSettingResourceKind(s.Type)
	token := strings.TrimSpace(s.Token)
	if !ok || token == "" {
		return ""
	}

	brand := core.LarkBrand("")
	if runtime != nil && runtime.Config != nil {
		brand = runtime.Config.Brand
	}
	host := "https://www.feishu.cn"
	if brand == core.BrandLark {
		host = "https://www.larksuite.com"
	}
	return host + resourceKind.CanonicalPath + url.PathEscape(token)
}

func validateDrivePermissionGetSettingToken(token string) error {
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

func (s drivePermissionGetSettingSpec) params() map[string]interface{} {
	return map[string]interface{}{"type": s.Type}
}

func (s drivePermissionGetSettingSpec) apiPath() string {
	return drivePermissionPublicV2Path(s.Token)
}

func drivePermissionPublicV2Path(token string) string {
	return fmt.Sprintf("/open-apis/drive/v2/permissions/%s/public", validate.EncodePathSegment(token))
}

func drivePermissionGetSettingPermissionPublic(data map[string]interface{}) (map[string]interface{}, error) {
	permissionPublic := common.GetMap(data, "permission_public")
	if permissionPublic == nil {
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"drive permission get response missing data.permission_public",
		)
	}
	return permissionPublic, nil
}

// DrivePermissionGetSetting queries permission_public settings for a Drive resource.
var DrivePermissionGetSetting = common.Shortcut{
	Service:     "drive",
	Command:     "+permission-get-setting",
	Description: "Get public access, sharing, collaborator management, security, and comment permission settings",
	Risk:        "read",
	Scopes:      []string{"docs:permission.setting:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "token", Desc: "target URL or bare token (doc/sheet/file/wiki/bitable/docx/mindnote/minutes/slides/folder/apps)", Required: true},
		{Name: "type", Desc: "target type; auto-inferred from URL, required for bare tokens", Enum: drivePermissionGetSettingTypes},
	},
	Tips: []string{
		"--token accepts a Lark URL or bare token; pass --type when using a bare token.",
		"Use --type folder for Drive folders. This shortcut reads the target's own permission settings; it does not recurse into child documents.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDrivePermissionGetSettingSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDrivePermissionGetSettingSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			Desc("Get Drive permission settings").
			GET(spec.apiPath()).
			Params(spec.params())
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDrivePermissionGetSettingSpec(runtime)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPITyped(
			"GET",
			spec.apiPath(),
			spec.params(),
			nil,
		)
		if err != nil {
			return err
		}

		permissionPublic, err := drivePermissionGetSettingPermissionPublic(data)
		if err != nil {
			return err
		}
		permissionPublicPretty, err := json.MarshalIndent(permissionPublic, "", "  ")
		if err != nil {
			return errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"encode drive permission settings for pretty output",
			).WithCause(err)
		}

		out := map[string]interface{}{"permission_public": permissionPublic}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Type:  %s\n", spec.Type)
			fmt.Fprintf(w, "Token: %s\n", spec.Token)
			if url := spec.url(runtime); url != "" {
				fmt.Fprintf(w, "URL:   %s\n", url)
			}
			fmt.Fprintf(w, "Permission settings:\n%s\n", permissionPublicPretty)
		})
		return nil
	},
}
