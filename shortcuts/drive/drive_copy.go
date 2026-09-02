// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	driveCopyMaxNameBytes = 256
	// driveCopyMySpaceSentinel lets callers target the My Space root folder
	// without knowing its token; Execute resolves it via the root-folder-meta
	// endpoint (absent from platform metadata, path fixed per official docs).
	driveCopyMySpaceSentinel    = "my_space"
	driveCopyRootFolderMetaPath = "/open-apis/drive/explorer/v2/root_folder/meta"
)

var driveCopyTypes = []string{"doc", "docx", "sheet", "file", "mindnote", "slides", "bitable", "base", "wiki"}

type driveCopyRef struct {
	Token      string
	Type       string
	SourceFlag string
	WikiToken  string
}

type driveCopyExtra struct {
	Key   string
	Value string
}

type driveCopySpec struct {
	Ref           driveCopyRef
	Name          string
	FolderToken   string // empty when FolderMySpace is set
	FolderMySpace bool
	Extras        []driveCopyExtra
}

// DriveCopy copies a Drive file into a target folder through the Drive copy
// API, with URL parsing. Wiki inputs are unwrapped first and the resolved
// resource type is passed through to the same Drive copy API.
var DriveCopy = common.Shortcut{
	Service:              "drive",
	Command:              "+copy",
	Description:          "Copy a Drive resource into a target folder; wiki token/URL sources are unwrapped automatically",
	Risk:                 "write",
	Scopes:               []string{"docs:document:copy"},
	ConditionalScopes:    []string{"drive:drive.metadata:readonly", "wiki:node:retrieve"},
	ConditionalBotScopes: []string{"drive:drive.metadata:readonly", "wiki:node:retrieve", "docs:permission.member:create"},
	AuthTypes:            []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "recommended: Lark/Feishu document URL, including wiki URLs"},
		{Name: "token", Desc: "document token or document URL; bare tokens require --type"},
		{Name: "type", Desc: "document type for bare --token; optional for URLs but must match the URL type when provided", Enum: driveCopyTypes},
		{Name: "name", Desc: "name for the copied file, up to 256 bytes", Required: true},
		{Name: "folder-token", Desc: "target folder token, folder URL, or the constant my_space to copy into the caller's My Space root folder", Required: true},
		{Name: "extra", Type: "string_array", Desc: "repeatable key=value pair forwarded verbatim as a custom copy parameter, e.g. --extra target_type=docx to convert a legacy doc into a docx copy"},
	},
	Tips: []string{
		"The source type must match the real file type; the API rejects mismatches.",
		"A wiki URL or --token with --type wiki is resolved through wiki get_node, then its underlying Drive resource is copied.",
		"Use `--extra target_type=docx` with a legacy doc source to create the copy as a new-version docx.",
		"`--folder-token my_space` resolves the caller's My Space root folder automatically; resolution needs the drive:drive.metadata:readonly (or drive:drive) scope.",
		"In bot mode, the CLI also tries to grant the current CLI user full_access on the new copy; the outcome is reported in the permission_grant output field.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveCopySpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveCopySpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		d := buildDriveCopyDryRun(spec)
		if runtime.IsBot() {
			d.Set("post_copy_note", "After the copy succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new copy.")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveCopySpec(runtime)
		if err != nil {
			return err
		}
		if spec.Ref.Type == "wiki" {
			spec, err = resolveDriveCopyWikiResource(ctx, runtime, spec)
			if err != nil {
				return err
			}
		}

		folderToken := spec.FolderToken
		if spec.FolderMySpace {
			folderToken, err = resolveDriveCopyMySpaceRoot(runtime)
			if err != nil {
				return err
			}
		}

		data, err := runtime.CallAPITyped(
			"POST",
			fmt.Sprintf("/open-apis/drive/v1/files/%s/copy", validate.EncodePathSegment(spec.Ref.Token)),
			nil,
			buildDriveCopyBody(spec, folderToken),
		)
		if err != nil {
			return err
		}

		copiedToken := common.GetString(data, "file", "token")
		if copiedToken == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "drive copy succeeded but returned no file token (data.file.token)")
		}
		out := buildDriveCopyOutput(runtime, spec, folderToken, data)
		copiedType := common.GetString(data, "file", "type")
		if copiedType == "" {
			copiedType = spec.Ref.Type
		}
		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, copiedToken, copiedType); grant != nil {
			out["permission_grant"] = grant
		}
		runtime.Out(out, nil)
		return nil
	},
}

func readDriveCopySpec(runtime *common.RuntimeContext) (driveCopySpec, error) {
	ref, err := resolveDriveCopyInput(runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveCopySpec{}, err
	}
	spec := driveCopySpec{
		Ref:  ref,
		Name: strings.TrimSpace(runtime.Str("name")),
	}
	spec.FolderToken, spec.FolderMySpace, err = resolveDriveCopyFolderToken(runtime.Str("folder-token"))
	if err != nil {
		return driveCopySpec{}, err
	}
	spec.Extras, err = parseDriveCopyExtras(runtime.StrArray("extra"))
	if err != nil {
		return driveCopySpec{}, err
	}
	if err := validateDriveCopySpec(spec); err != nil {
		return driveCopySpec{}, err
	}
	return spec, nil
}

// parseDriveCopyExtras converts repeated `key=value` specs into the API's
// extra parameter shape, preserving order and transcribing values verbatim.
func parseDriveCopyExtras(specs []string) ([]driveCopyExtra, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	extras := make([]driveCopyExtra, 0, len(specs))
	for _, spec := range specs {
		key, value, found := strings.Cut(spec, "=")
		if !found {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --extra %q: expected format key=value", spec).WithParam("--extra")
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --extra %q: key must not be empty", spec).WithParam("--extra")
		}
		if value == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --extra %q: value must not be empty", spec).WithParam("--extra")
		}
		extras = append(extras, driveCopyExtra{Key: key, Value: value})
	}
	return extras, nil
}

func validateDriveCopySpec(spec driveCopySpec) error {
	if spec.Name == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--name must not be empty or whitespace-only").WithParam("--name")
	}
	if len(spec.Name) > driveCopyMaxNameBytes {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--name exceeds %d bytes (got %d)", driveCopyMaxNameBytes, len(spec.Name)).WithParam("--name")
	}
	return nil
}

func resolveDriveCopyInput(urlInput, tokenInput, explicitType string) (driveCopyRef, error) {
	urlInput = strings.TrimSpace(urlInput)
	tokenInput = strings.TrimSpace(tokenInput)
	if urlInput != "" && tokenInput != "" {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url and --token are mutually exclusive; pass one input only").WithParam("--url")
	}
	if urlInput == "" && tokenInput == "" {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --url or --token").WithParam("--url")
	}

	raw := urlInput
	sourceFlag := "--url"
	if raw == "" {
		raw = tokenInput
		sourceFlag = "--token"
	}
	inputType := normalizeDriveCopyType(strings.ToLower(strings.TrimSpace(explicitType)))

	if ref, ok := common.ParseResourceURL(raw); ok {
		refType := normalizeDriveCopyType(ref.Type)
		if inputType != "" && inputType != refType {
			return driveCopyRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--type %q conflicts with URL path type %q; remove --type or use a matching value",
				inputType,
				refType,
			).WithParam("--type")
		}
		if !driveCopyTypeSupported(refType) {
			return driveCopyRef{}, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"unsupported %s resource type %q; drive copy supports doc, docx, sheet, file, mindnote, slides, bitable/base, and wiki documents",
				sourceFlag,
				refType,
			).WithParam(sourceFlag)
		}
		if err := validate.ResourceName(ref.Token, sourceFlag); err != nil {
			return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam(sourceFlag)
		}
		return driveCopyRef{Token: ref.Token, Type: refType, SourceFlag: sourceFlag}, nil
	}

	if strings.Contains(raw, "://") {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported %s URL %q: use a recognized Lark document URL or pass a bare token with --type", sourceFlag, raw).WithParam(sourceFlag)
	}
	if strings.ContainsAny(raw, "/?#") {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid bare token %q: remove path/query fragments or pass a recognized Lark document URL", raw).WithParam(sourceFlag)
	}
	if inputType == "" {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--type is required when %s is a bare token (allowed: doc, docx, sheet, file, mindnote, slides, bitable, base, wiki)", sourceFlag).WithParam("--type")
	}
	if !driveCopyTypeSupported(inputType) {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --type %q; allowed: doc, docx, sheet, file, mindnote, slides, bitable, base, wiki", inputType).WithParam("--type")
	}
	if err := validate.ResourceName(raw, sourceFlag); err != nil {
		return driveCopyRef{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam(sourceFlag)
	}
	return driveCopyRef{Token: raw, Type: inputType, SourceFlag: sourceFlag}, nil
}

func resolveDriveCopyWikiResource(ctx context.Context, runtime *common.RuntimeContext, spec driveCopySpec) (driveCopySpec, error) {
	wikiToken := spec.Ref.Token
	data, err := driveInspectCallWithRetry(ctx, func() (map[string]interface{}, error) {
		return runtime.CallAPITyped(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{"token": wikiToken},
			nil,
		)
	})
	if err != nil {
		return driveCopySpec{}, driveInspectAnnotateError("resolve_wiki", err)
	}

	node := common.GetMap(data, "node")
	objType := normalizeDriveCopyType(strings.ToLower(strings.TrimSpace(common.GetString(node, "obj_type"))))
	objToken := strings.TrimSpace(common.GetString(node, "obj_token"))
	if objType == "" || objToken == "" {
		return driveCopySpec{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki get_node returned incomplete node data (obj_type=%q, obj_token=%q)",
			objType,
			objToken,
		)
	}
	if !driveCopyAPITypeSupported(objType) {
		return driveCopySpec{}, errs.NewValidationError(
			errs.SubtypeFailedPrecondition,
			"wiki node resolves to unsupported Drive copy type %q",
			objType,
		).WithParam(spec.Ref.SourceFlag).WithHint("use a wiki node whose underlying type is doc, docx, sheet, file, mindnote, slides, or bitable")
	}
	if err := validate.ResourceName(objToken, spec.Ref.SourceFlag); err != nil {
		return driveCopySpec{}, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"wiki get_node returned an unsafe obj_token: %s",
			err,
		).WithCause(err)
	}

	spec.Ref.Token = objToken
	spec.Ref.Type = objType
	spec.Ref.WikiToken = wikiToken
	return spec, nil
}

func resolveDriveCopyFolderToken(input string) (string, bool, error) {
	input = strings.TrimSpace(input)
	if strings.EqualFold(input, driveCopyMySpaceSentinel) {
		return "", true, nil
	}
	if ref, ok := common.ParseResourceURL(input); ok {
		if ref.Type != "folder" {
			return "", false, errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--folder-token URL resolves to %q, not a folder; pass a folder URL, a folder token, or my_space",
				ref.Type,
			).WithParam("--folder-token")
		}
		if err := validate.ResourceName(ref.Token, "--folder-token"); err != nil {
			return "", false, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--folder-token")
		}
		return ref.Token, false, nil
	}
	if err := validate.ResourceName(input, "--folder-token"); err != nil {
		return "", false, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--folder-token")
	}
	return input, false, nil
}

// resolveDriveCopyMySpaceRoot fetches the caller's My Space root folder token.
// The endpoint is absent from platform metadata; the path follows the official
// get-root-folder-meta documentation and works for both user and bot tokens.
func resolveDriveCopyMySpaceRoot(runtime *common.RuntimeContext) (string, error) {
	data, err := runtime.CallAPITyped("GET", driveCopyRootFolderMetaPath, nil, nil)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(common.GetString(data, "token"))
	if token == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "root folder meta returned an empty token")
	}
	return token, nil
}

func normalizeDriveCopyType(docType string) string {
	switch strings.TrimSpace(docType) {
	case "base":
		return "bitable"
	default:
		return strings.TrimSpace(docType)
	}
}

func driveCopyTypeSupported(docType string) bool {
	return normalizeDriveCopyType(docType) == "wiki" || driveCopyAPITypeSupported(docType)
}

func driveCopyAPITypeSupported(docType string) bool {
	switch normalizeDriveCopyType(docType) {
	case "doc", "docx", "sheet", "file", "mindnote", "slides", "bitable":
		return true
	default:
		return false
	}
}

func buildDriveCopyBody(spec driveCopySpec, folderToken string) map[string]interface{} {
	body := map[string]interface{}{
		"name":         spec.Name,
		"type":         spec.Ref.Type,
		"folder_token": folderToken,
	}
	if len(spec.Extras) > 0 {
		extras := make([]map[string]interface{}, 0, len(spec.Extras))
		for _, extra := range spec.Extras {
			extras = append(extras, map[string]interface{}{"key": extra.Key, "value": extra.Value})
		}
		body["extra"] = extras
	}
	return body
}

func buildDriveCopyDryRun(spec driveCopySpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		dry := common.NewDryRunAPI().
			Desc("Resolve wiki node, then copy its underlying resource into Drive").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve the wiki node to its underlying resource").
			Params(map[string]interface{}{"token": spec.Ref.Token})
		folderToken := spec.FolderToken
		copyStep := 2
		if spec.FolderMySpace {
			dry.GET(driveCopyRootFolderMetaPath).
				Desc("[2] Resolve the caller's My Space root folder token")
			folderToken = "<root folder token from step 2>"
			copyStep = 3
		}
		resolved := spec
		resolved.Ref.Token = "<obj_token from step 1>"
		resolved.Ref.Type = "<supported obj_type from step 1>"
		return dry.POST("/open-apis/drive/v1/files/<obj_token from step 1>/copy").
			Desc(fmt.Sprintf("[%d] Copy the resolved wiki resource into Drive", copyStep)).
			Body(buildDriveCopyBody(resolved, folderToken)).
			Set("wiki_token", spec.Ref.Token).
			Set("wiki_source_constraint", "obj_type must be supported by Drive copy")
	}
	if spec.FolderMySpace {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve My Space root -> copy").
			GET(driveCopyRootFolderMetaPath).
			Desc("[1] Resolve the caller's My Space root folder token").
			POST("/open-apis/drive/v1/files/:file_token/copy").
			Desc("[2] Copy file into the resolved root folder").
			Body(buildDriveCopyBody(spec, "<root folder token from step 1>")).
			Set("file_token", spec.Ref.Token)
	}
	return common.NewDryRunAPI().
		Desc("1-step request: copy file into target folder").
		POST("/open-apis/drive/v1/files/:file_token/copy").
		Body(buildDriveCopyBody(spec, spec.FolderToken)).
		Set("file_token", spec.Ref.Token)
}

func buildDriveCopyOutput(runtime *common.RuntimeContext, spec driveCopySpec, folderToken string, data map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"copied":            true,
		"source_file_token": spec.Ref.Token,
		"source_type":       spec.Ref.Type,
		"folder_token":      folderToken,
	}
	if spec.Ref.WikiToken != "" {
		out["source_wiki_token"] = spec.Ref.WikiToken
	}
	file := common.GetMap(data, "file")
	if token := common.GetString(file, "token"); token != "" {
		out["file_token"] = token
		if url := common.GetString(file, "url"); url != "" {
			out["url"] = url
		} else if built := common.BuildResourceURL(runtime.Config.Brand, common.GetString(file, "type"), token); built != "" {
			out["url"] = built
		}
	}
	if fileType := common.GetString(file, "type"); fileType != "" {
		out["file_type"] = fileType
	}
	if name := common.GetString(file, "name"); name != "" {
		out["name"] = name
	}
	return out
}
