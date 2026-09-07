// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type driveMemberListSpec struct {
	Token    string
	Type     string
	Fields   string
	PermType string
}

var driveMemberListTypes = []string{
	"doc", "sheet", "file", "wiki", "bitable", "docx",
	"mindnote", "minutes", "slides", "folder", "apps",
}

var driveMemberListFields = []string{"name", "type", "avatar", "external_label"}
var driveMemberListPermTypes = []string{"container", "single_page"}

var driveMemberListURLPathToType = []struct {
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
	{"/page/", "apps"},
}

func readDriveMemberListSpec(runtime *common.RuntimeContext) (driveMemberListSpec, error) {
	token, resourceType, err := resolveDriveMemberListTarget(runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveMemberListSpec{}, err
	}
	fields, err := normalizeDriveMemberListFields(runtime.Str("fields"), runtime.Changed("fields"))
	if err != nil {
		return driveMemberListSpec{}, err
	}
	permType, err := normalizeDriveMemberListPermType(runtime.Str("perm-type"), resourceType, runtime.Changed("perm-type"))
	if err != nil {
		return driveMemberListSpec{}, err
	}
	return driveMemberListSpec{
		Token:    token,
		Type:     resourceType,
		Fields:   fields,
		PermType: permType,
	}, nil
}

func resolveDriveMemberListTarget(raw, explicitType string) (token, resourceType string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token is required").WithParam("--token")
	}

	explicitType, err = normalizeDriveMemberListEnumValue(explicitType, driveMemberListTypes, "--type")
	if err != nil {
		return "", "", err
	}

	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil || parsed.Hostname() == "" {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--token URL is malformed: %q", raw).WithParam("--token")
		}
		ref, ok := parseDriveMemberListResourceURLPath(parsed.Path)
		if !ok {
			return "", "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported --token URL %q: pass a recognized Lark Drive resource URL or a bare token with --type",
				raw,
			).WithParam("--token")
		}
		if explicitType != "" && explicitType != ref.Type {
			return "", "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				explicitType,
				ref.Type,
			).WithParam("--type")
		}
		if err := validate.ResourceName(ref.Token, "--token"); err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--token")
		}
		return ref.Token, ref.Type, nil
	}

	if explicitType == "" {
		return "", "", errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--type is required when --token is a bare token; accepted values: %s",
			strings.Join(driveMemberListTypes, ", "),
		).WithParam("--type")
	}
	if err := validate.ResourceName(raw, "--token"); err != nil {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--token")
	}
	return raw, explicitType, nil
}

func parseDriveMemberListResourceURLPath(path string) (common.ResourceRef, bool) {
	for _, mapping := range driveMemberListURLPathToType {
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
			return common.ResourceRef{}, false
		}
		return common.ResourceRef{Type: mapping.Type, Token: token}, true
	}
	return common.ResourceRef{}, false
}

func normalizeDriveMemberListFields(raw string, changed bool) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if changed {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--fields cannot be blank; allowed: %s, *", strings.Join(driveMemberListFields, ", ")).WithParam("--fields")
		}
		return "", nil
	}

	parts := strings.Split(raw, ",")
	fields := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		field := strings.ToLower(strings.TrimSpace(part))
		if field == "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--fields contains an empty field; allowed: %s, *", strings.Join(driveMemberListFields, ", ")).WithParam("--fields")
		}
		if field == "*" {
			if len(parts) != 1 {
				return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--fields=* cannot be combined with other fields").WithParam("--fields")
			}
			return "*", nil
		}
		if !driveMemberListFieldAllowed(field) {
			return "", errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"invalid value %q for --fields, allowed: %s, *",
				strings.TrimSpace(part),
				strings.Join(driveMemberListFields, ", "),
			).WithParam("--fields")
		}
		if !seen[field] {
			fields = append(fields, field)
			seen[field] = true
		}
	}
	return strings.Join(fields, ","), nil
}

func driveMemberListFieldAllowed(field string) bool {
	for _, allowed := range driveMemberListFields {
		if field == allowed {
			return true
		}
	}
	return false
}

func normalizeDriveMemberListPermType(raw, resourceType string, changed bool) (string, error) {
	permType, err := normalizeDriveMemberListEnumValue(raw, driveMemberListPermTypes, "--perm-type")
	if err != nil {
		return "", err
	}
	if resourceType != "wiki" && changed {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--perm-type only applies when resource type is wiki; got %q", resourceType).WithParam("--perm-type")
	}
	return permType, nil
}

func normalizeDriveMemberListEnumValue(raw string, allowed []string, flagName string) (string, error) {
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

func (s driveMemberListSpec) apiPath() string {
	return fmt.Sprintf("/open-apis/drive/v1/permissions/%s/members", validate.EncodePathSegment(s.Token))
}

func (s driveMemberListSpec) params() map[string]interface{} {
	params := map[string]interface{}{"type": s.Type}
	if s.Fields != "" {
		params["fields"] = s.Fields
	}
	if s.PermType != "" {
		params["perm_type"] = s.PermType
	}
	return params
}

// DriveMemberList lists collaborator/member permissions on a Drive resource.
var DriveMemberList = common.Shortcut{
	Service:     "drive",
	Command:     "+member-list",
	Description: "List collaborator/member permissions on a Drive resource",
	Risk:        "read",
	Scopes:      []string{"docs:permission.member:retrieve"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "token", Desc: "target URL or bare token (doc/sheet/file/wiki/bitable/docx/mindnote/minutes/slides/folder/apps)", Required: true},
		{Name: "type", Desc: "target type; auto-inferred from URL, required for bare tokens"},
		{Name: "fields", Desc: "optional collaborator fields to return: name,type,avatar,external_label or *"},
		{Name: "perm-type", Desc: "wiki permission scope filter; one of container|single_page"},
	},
	Tips: []string{
		"--token accepts a Lark URL or bare token; pass --type when using a bare token.",
		"Use --type folder for Drive folders.",
		"--fields is omitted by default; pass --fields '*' or a comma-separated subset when extra collaborator fields are needed.",
		"--perm-type only applies to wiki nodes.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveMemberListSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveMemberListSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			Desc("List Drive collaborator/member permissions").
			GET(spec.apiPath()).
			Params(spec.params())
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveMemberListSpec(runtime)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPITyped("GET", spec.apiPath(), spec.params(), nil)
		if err != nil {
			return err
		}
		runtime.OutFormat(data, nil, func(w io.Writer) {
			renderDriveMemberListPretty(w, data)
		})
		return nil
	},
}

func renderDriveMemberListPretty(w io.Writer, data map[string]interface{}) {
	items, _ := data["items"].([]interface{})
	if len(items) == 0 {
		fmt.Fprintln(w, "No Drive members found.")
		return
	}
	for i, raw := range items {
		member, _ := raw.(map[string]interface{})
		fmt.Fprintf(w, "[%d] %s\n", i+1, driveMemberListValue(member["member_id"]))
		fmt.Fprintf(w, "    member_type: %s\n", driveMemberListValue(member["member_type"]))
		fmt.Fprintf(w, "    perm:        %s\n", driveMemberListValue(member["perm"]))
		if permType := driveMemberListValue(member["perm_type"]); permType != "-" {
			fmt.Fprintf(w, "    perm_type:   %s\n", permType)
		}
		if memberType := driveMemberListValue(member["type"]); memberType != "-" {
			fmt.Fprintf(w, "    type:        %s\n", memberType)
		}
		if name := driveMemberListValue(member["name"]); name != "-" {
			fmt.Fprintf(w, "    name:        %s\n", name)
		}
		if avatar := driveMemberListValue(member["avatar"]); avatar != "-" {
			fmt.Fprintf(w, "    avatar:      %s\n", avatar)
		}
		if label, ok := member["external_label"]; ok {
			fmt.Fprintf(w, "    external_label: %v\n", label)
		}
		fmt.Fprintln(w)
	}
}

func driveMemberListValue(v interface{}) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return "-"
}
