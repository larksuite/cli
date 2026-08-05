// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	memberListPath        = apiBasePath + "/apps/%s/members"
	memberRemovePath      = apiBasePath + "/apps/%s/members/remove"
	memberSettingsPath    = apiBasePath + "/apps/%s/member-settings"
	defaultMemberPageSize = 20
	maxMemberPageSize     = 100
)

var (
	memberWriteTypes = []string{"openid", "openchat", "opendepartmentid"}
	memberListTypes  = []string{"user", "department", "chat"}
	memberRoles      = []string{"view", "edit", "full_access"}
)

func normalizeMemberAPIError(err error) error {
	if err == nil {
		return nil
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	switch problem.Code {
	case 40004, 3340004:
		problem.Subtype = errs.SubtypeInvalidParameters
		problem.Hint = "discard --page-token and restart from the first page"
	case 40005, 3340005:
		problem.Subtype = errs.SubtypeFeatureNotAvailable
		problem.Message = "Collaborator management is not available for this app via lark-cli."
		problem.Hint = "Open this app in Miaoda and manage collaborators from its permission settings."
		problem.Retryable = false
	case 40400, 3340400:
		problem.Subtype = errs.SubtypeNotFound
	}
	return err
}

type memberIdentityRequest struct {
	UserOpenID   string `json:"user_open_id,omitempty"`
	DepartmentID string `json:"department_id,omitempty"`
	ChatID       string `json:"chat_id,omitempty"`
}

type memberAddRequest struct {
	memberIdentityRequest
	Role             string `json:"role"`
	NeedNotification *bool  `json:"need_notification,omitempty"`
}

type memberUpdateRequest struct {
	memberIdentityRequest
	Role string `json:"role"`
}

type memberRemoveRequest struct {
	memberIdentityRequest
}

type memberSettingsUpdateRequest struct {
	ExternalAccess        *string `json:"external_access,omitempty"`
	ExternalInvite        *string `json:"external_invite,omitempty"`
	LinkShare             *string `json:"link_share,omitempty"`
	ManageCollaboratorsBy *string `json:"manage_collaborators_by,omitempty"`
	CommentBy             *string `json:"comment_by,omitempty"`
	CopyDownloadBy        *string `json:"copy_download_by,omitempty"`
}

type memberAPIRecord struct {
	MemberType   string  `json:"member_type"`
	UserOpenID   *string `json:"user_open_id,omitempty"`
	DepartmentID *string `json:"department_id,omitempty"`
	ChatID       *string `json:"chat_id,omitempty"`
	Name         string  `json:"name,omitempty"`
	Role         string  `json:"role"`
}

type memberOutput struct {
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
	Name       string `json:"name,omitempty"`
	Role       string `json:"role"`
}

type memberSettingsResponse struct {
	ExternalAccess        *string `json:"external_access,omitempty"`
	ExternalInvite        *string `json:"external_invite,omitempty"`
	LinkShare             *string `json:"link_share,omitempty"`
	ManageCollaboratorsBy *string `json:"manage_collaborators_by,omitempty"`
	CommentBy             *string `json:"comment_by,omitempty"`
	CopyDownloadBy        *string `json:"copy_download_by,omitempty"`
}

type memberSettingSpec struct {
	flag          string
	field         string
	description   string
	allowed       []string
	setRequest    func(*memberSettingsUpdateRequest, *string)
	responseValue func(memberSettingsResponse) *string
}

var memberSettingSpecs = []memberSettingSpec{
	{
		flag: "external-access", field: "external_access", description: "external sharing",
		allowed:       []string{"enabled", "disabled"},
		setRequest:    func(req *memberSettingsUpdateRequest, value *string) { req.ExternalAccess = value },
		responseValue: func(settings memberSettingsResponse) *string { return settings.ExternalAccess },
	},
	{
		flag: "external-invite", field: "external_invite", description: "external collaborator invitations",
		allowed:       []string{"enabled", "disabled"},
		setRequest:    func(req *memberSettingsUpdateRequest, value *string) { req.ExternalInvite = value },
		responseValue: func(settings memberSettingsResponse) *string { return settings.ExternalInvite },
	},
	{
		flag: "link-share", field: "link_share", description: "link sharing",
		allowed:       []string{"closed", "tenant-readable", "tenant-editable", "anyone-readable"},
		setRequest:    func(req *memberSettingsUpdateRequest, value *string) { req.LinkShare = value },
		responseValue: func(settings memberSettingsResponse) *string { return settings.LinkShare },
	},
	{
		flag: "manage-collaborators-by", field: "manage_collaborators_by", description: "who can manage collaborators",
		allowed:       []string{"anyone", "same-tenant", "full-access"},
		setRequest:    func(req *memberSettingsUpdateRequest, value *string) { req.ManageCollaboratorsBy = value },
		responseValue: func(settings memberSettingsResponse) *string { return settings.ManageCollaboratorsBy },
	},
	{
		flag: "comment-by", field: "comment_by", description: "who can comment",
		allowed:       []string{"viewer", "editor"},
		setRequest:    func(req *memberSettingsUpdateRequest, value *string) { req.CommentBy = value },
		responseValue: func(settings memberSettingsResponse) *string { return settings.CommentBy },
	},
	{
		flag: "copy-download-by", field: "copy_download_by", description: "who can copy, print, or download",
		allowed:       []string{"viewer", "editor", "full-access"},
		setRequest:    func(req *memberSettingsUpdateRequest, value *string) { req.CopyDownloadBy = value },
		responseValue: func(settings memberSettingsResponse) *string { return settings.CopyDownloadBy },
	},
}

type memberSettingChangeResponse struct {
	Field  string  `json:"field"`
	Before *string `json:"before,omitempty"`
	After  *string `json:"after,omitempty"`
}

type memberListAPIResponse struct {
	Items     *[]memberAPIRecord `json:"items"`
	HasMore   bool               `json:"has_more"`
	PageToken string             `json:"page_token,omitempty"`
}

type memberListOutput struct {
	Items     []memberOutput `json:"items"`
	HasMore   bool           `json:"has_more"`
	PageToken string         `json:"page_token,omitempty"`
}

type memberAddAPIResponse struct {
	Member  *memberAPIRecord `json:"member"`
	Changed bool             `json:"changed"`
}

type memberAddOutput struct {
	Member  memberOutput `json:"member"`
	Changed bool         `json:"changed"`
}

type memberUpdateAPIResponse struct {
	Member     *memberAPIRecord `json:"member"`
	BeforeRole string           `json:"before_role"`
	AfterRole  string           `json:"after_role"`
	Changed    bool             `json:"changed"`
}

type memberUpdateOutput struct {
	Member     memberOutput `json:"member"`
	BeforeRole string       `json:"before_role"`
	AfterRole  string       `json:"after_role"`
	Changed    bool         `json:"changed"`
}

type memberRemoveAPIResponse struct {
	Member  *memberAPIRecord `json:"member"`
	Changed bool             `json:"changed"`
}

type memberRemoveOutput struct {
	Member  memberOutput `json:"member"`
	Changed bool         `json:"changed"`
}

type memberSettingsGetAPIResponse struct {
	Settings *memberSettingsResponse `json:"settings"`
}

type memberSettingsGetOutput struct {
	Settings memberSettingsResponse `json:"settings"`
}

type memberSettingsSetAPIResponse struct {
	Settings *memberSettingsResponse        `json:"settings"`
	Changes  *[]memberSettingChangeResponse `json:"changes"`
	Changed  bool                           `json:"changed"`
}

type memberSettingsSetOutput struct {
	Settings memberSettingsResponse        `json:"settings"`
	Changes  []memberSettingChangeResponse `json:"changes"`
	Changed  bool                          `json:"changed"`
}

func memberAppID(rctx *common.RuntimeContext) string {
	return strings.TrimSpace(rctx.Str("app-id"))
}

func validateMemberAppID(rctx *common.RuntimeContext) error {
	appID := memberAppID(rctx)
	if appID == "" {
		return appsValidationParamError("--app-id", "--app-id is required").
			WithHint("list your Miaoda apps with `lark-cli apps +list`")
	}
	if strings.HasPrefix(appID, "cli_") {
		return appsValidationParamError("--app-id", "--app-id must be a Miaoda app_id, not a credential app id").
			WithHint("pass the app_... value returned by `lark-cli apps +list`, not the cli_... credential app id")
	}
	if !strings.HasPrefix(appID, "app_") || len(appID) == len("app_") {
		return appsValidationParamError("--app-id", "--app-id must start with app_ and include an identifier").
			WithHint("list your Miaoda apps with `lark-cli apps +list`, then pass its app_id")
	}
	for _, r := range appID {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '/' || r == '\\' {
			return appsValidationParamError("--app-id", "--app-id must not contain slashes, whitespace, or control characters").
				WithHint("pass one app_... identifier exactly as returned by `lark-cli apps +list`")
		}
	}
	if err := validate.ResourceName(appID, "--app-id"); err != nil {
		return appsValidationParamError("--app-id", "invalid --app-id: %v", err).
			WithCause(err).
			WithHint("pass one app_... identifier exactly as returned by `lark-cli apps +list`")
	}
	return nil
}

func buildMemberListParams(rctx *common.RuntimeContext) (map[string]interface{}, error) {
	pageSize := rctx.Int("page-size")
	if pageSize == 0 && !rctx.Changed("page-size") {
		pageSize = defaultMemberPageSize
	}
	if pageSize < 1 || pageSize > maxMemberPageSize {
		return nil, appsValidationParamError("--page-size", "--page-size must be between 1 and %d", maxMemberPageSize).
			WithHint("use --page-size between 1 and 100")
	}

	params := map[string]interface{}{"page_size": pageSize}
	if role := strings.TrimSpace(rctx.Str("role")); role != "" {
		if !memberStringAllowed(role, memberRoles) {
			return nil, appsValidationParamError("--role", "--role must be one of: view, edit, full_access").
				WithHint("omit --role to list every collaborator role")
		}
		params["role"] = role
	}
	if memberType := strings.TrimSpace(rctx.Str("member-type")); memberType != "" {
		if !memberStringAllowed(memberType, memberListTypes) {
			return nil, appsValidationParamError("--member-type", "--member-type must be one of: user, department, chat").
				WithHint("omit --member-type to list every collaborator type")
		}
		params["member_type"] = memberType
	}
	if pageToken := strings.TrimSpace(rctx.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	return params, nil
}

func buildMemberIdentity(memberType, memberID string) (memberIdentityRequest, error) {
	memberType = strings.TrimSpace(memberType)
	memberID = strings.TrimSpace(memberID)
	if !memberStringAllowed(memberType, memberWriteTypes) {
		return memberIdentityRequest{}, appsValidationParamError("--member-type", "--member-type must be one of: openid, openchat, opendepartmentid").
			WithHint("choose the type that matches the open ID you are passing")
	}
	if memberID == "" {
		return memberIdentityRequest{}, appsValidationParamError("--member-id", "--member-id is required").
			WithHint("resolve the collaborator to a user, chat, or department open ID first")
	}

	prefix := map[string]string{
		"openid":           "ou_",
		"openchat":         "oc_",
		"opendepartmentid": "od-",
	}[memberType]
	if !strings.HasPrefix(memberID, prefix) || len(memberID) == len(prefix) {
		return memberIdentityRequest{}, appsValidationParamError("--member-id", "--member-id for --member-type=%s must start with %s", memberType, prefix).
			WithHint(fmt.Sprintf("pass the matching external ID: %s...; internal numeric IDs are not accepted", prefix))
	}
	for _, r := range memberID {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("?#%/\\", r) {
			return memberIdentityRequest{}, appsValidationParamError("--member-id", "--member-id must not contain whitespace, control characters, or URL metacharacters").
				WithHint("pass one external open ID exactly as returned by Lark")
		}
	}

	switch memberType {
	case "openid":
		return memberIdentityRequest{UserOpenID: memberID}, nil
	case "openchat":
		return memberIdentityRequest{ChatID: memberID}, nil
	case "opendepartmentid":
		return memberIdentityRequest{DepartmentID: memberID}, nil
	default:
		return memberIdentityRequest{}, appsValidationParamError("--member-type", "--member-type has no typed request mapping").
			WithHint("choose one of: openid, openchat, opendepartmentid")
	}
}

func memberListURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(memberListPath, validate.EncodePathSegment(memberAppID(rctx)))
}

func memberRemoveURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(memberRemovePath, validate.EncodePathSegment(memberAppID(rctx)))
}

func memberSettingsURL(rctx *common.RuntimeContext) string {
	return fmt.Sprintf(memberSettingsPath, validate.EncodePathSegment(memberAppID(rctx)))
}

func buildMemberAddRequest(rctx *common.RuntimeContext) (memberAddRequest, error) {
	identity, err := buildMemberIdentity(rctx.Str("member-type"), rctx.Str("member-id"))
	if err != nil {
		return memberAddRequest{}, err
	}
	req := memberAddRequest{
		memberIdentityRequest: identity,
		Role:                  strings.TrimSpace(rctx.Str("perm")),
	}
	if rctx.Changed("need-notification") {
		needNotification := rctx.Bool("need-notification")
		req.NeedNotification = &needNotification
	}
	return req, nil
}

func buildMemberUpdateRequest(rctx *common.RuntimeContext) (memberUpdateRequest, error) {
	identity, err := buildMemberIdentity(rctx.Str("member-type"), rctx.Str("member-id"))
	if err != nil {
		return memberUpdateRequest{}, err
	}
	return memberUpdateRequest{
		memberIdentityRequest: identity,
		Role:                  strings.TrimSpace(rctx.Str("perm")),
	}, nil
}

func buildMemberRemoveRequest(rctx *common.RuntimeContext) (memberRemoveRequest, error) {
	identity, err := buildMemberIdentity(rctx.Str("member-type"), rctx.Str("member-id"))
	if err != nil {
		return memberRemoveRequest{}, err
	}
	return memberRemoveRequest{memberIdentityRequest: identity}, nil
}

func buildMemberSettingsUpdateRequest(rctx *common.RuntimeContext) (memberSettingsUpdateRequest, error) {
	var req memberSettingsUpdateRequest
	for _, spec := range memberSettingSpecs {
		if !rctx.Changed(spec.flag) {
			continue
		}
		value := strings.TrimSpace(rctx.Str(spec.flag))
		if !memberStringAllowed(value, spec.allowed) {
			return memberSettingsUpdateRequest{}, appsValidationParamError("--"+spec.flag, "invalid value %q for --%s", value, spec.flag).
				WithHint("choose one of the documented setting values shown by --help")
		}
		spec.setRequest(&req, &value)
	}
	return req, nil
}

func validateMemberMutation(rctx *common.RuntimeContext, requirePerm bool) error {
	if err := validateMemberAppID(rctx); err != nil {
		return err
	}
	if _, err := buildMemberIdentity(rctx.Str("member-type"), rctx.Str("member-id")); err != nil {
		return err
	}
	if requirePerm {
		perm := strings.TrimSpace(rctx.Str("perm"))
		if !memberStringAllowed(perm, memberRoles) {
			return appsValidationParamError("--perm", "--perm must be one of: view, edit, full_access").
				WithHint("choose the collaborator permission explicitly")
		}
	}
	return nil
}

func validateMemberSettingsSet(rctx *common.RuntimeContext) error {
	if err := validateMemberAppID(rctx); err != nil {
		return err
	}
	for _, spec := range memberSettingSpecs {
		if rctx.Changed(spec.flag) {
			_, err := buildMemberSettingsUpdateRequest(rctx)
			return err
		}
	}
	return appsValidationError("at least one collaborator setting must be provided").
		WithParams(
			appsInvalidParam("--external-access", "not provided"),
			appsInvalidParam("--external-invite", "not provided"),
			appsInvalidParam("--link-share", "not provided"),
			appsInvalidParam("--manage-collaborators-by", "not provided"),
			appsInvalidParam("--comment-by", "not provided"),
			appsInvalidParam("--copy-download-by", "not provided"),
		).
		WithHint("pass at least one setting flag; omitted settings remain unchanged")
}

func memberStringAllowed(value string, allowed []string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func decodeMemberAPIData(data map[string]interface{}, out interface{}) error {
	if data == nil {
		return memberInvalidResponse("member API response data must be an object")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return memberInvalidResponse("member API response could not be decoded").WithCause(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return memberInvalidResponse("member API response has an invalid shape").WithCause(err)
	}
	return nil
}

func projectMemberListData(data map[string]interface{}) (memberListOutput, error) {
	var decoded memberListAPIResponse
	if err := decodeMemberAPIData(data, &decoded); err != nil {
		return memberListOutput{}, err
	}
	if decoded.Items == nil {
		return memberListOutput{}, memberInvalidResponse("member list response is missing items")
	}
	if decoded.HasMore && strings.TrimSpace(decoded.PageToken) == "" {
		return memberListOutput{}, memberInvalidResponse("member list response has_more=true but page_token is missing")
	}
	items := make([]memberOutput, 0, len(*decoded.Items))
	for _, raw := range *decoded.Items {
		item, err := projectMemberRecord(raw)
		if err != nil {
			return memberListOutput{}, err
		}
		items = append(items, item)
	}
	return memberListOutput{
		Items:     items,
		HasMore:   decoded.HasMore,
		PageToken: decoded.PageToken,
	}, nil
}

func projectMemberAddData(data map[string]interface{}) (memberAddOutput, error) {
	var decoded memberAddAPIResponse
	if err := decodeMemberAPIData(data, &decoded); err != nil {
		return memberAddOutput{}, err
	}
	if decoded.Member == nil {
		return memberAddOutput{}, memberInvalidResponse("member add response is missing member")
	}
	member, err := projectMemberRecord(*decoded.Member)
	if err != nil {
		return memberAddOutput{}, err
	}
	return memberAddOutput{Member: member, Changed: decoded.Changed}, nil
}

func projectMemberUpdateData(data map[string]interface{}) (memberUpdateOutput, error) {
	var decoded memberUpdateAPIResponse
	if err := decodeMemberAPIData(data, &decoded); err != nil {
		return memberUpdateOutput{}, err
	}
	if decoded.Member == nil {
		return memberUpdateOutput{}, memberInvalidResponse("member update response is missing member")
	}
	if !memberStringAllowed(decoded.BeforeRole, memberRoles) || !memberStringAllowed(decoded.AfterRole, memberRoles) {
		return memberUpdateOutput{}, memberInvalidResponse("member update response contains an unsupported role transition")
	}
	member, err := projectMemberRecord(*decoded.Member)
	if err != nil {
		return memberUpdateOutput{}, err
	}
	return memberUpdateOutput{
		Member: member, BeforeRole: decoded.BeforeRole, AfterRole: decoded.AfterRole, Changed: decoded.Changed,
	}, nil
}

func projectMemberRemoveData(data map[string]interface{}) (memberRemoveOutput, error) {
	var decoded memberRemoveAPIResponse
	if err := decodeMemberAPIData(data, &decoded); err != nil {
		return memberRemoveOutput{}, err
	}
	if decoded.Member == nil {
		return memberRemoveOutput{}, memberInvalidResponse("member remove response is missing member")
	}
	member, err := projectMemberRecord(*decoded.Member)
	if err != nil {
		return memberRemoveOutput{}, err
	}
	return memberRemoveOutput{Member: member, Changed: decoded.Changed}, nil
}

func projectMemberSettingsGetData(data map[string]interface{}) (memberSettingsGetOutput, error) {
	var decoded memberSettingsGetAPIResponse
	if err := decodeMemberAPIData(data, &decoded); err != nil {
		return memberSettingsGetOutput{}, err
	}
	if decoded.Settings == nil {
		return memberSettingsGetOutput{}, memberInvalidResponse("member settings response is missing settings")
	}
	if err := validateMemberSettingsResponse(*decoded.Settings); err != nil {
		return memberSettingsGetOutput{}, err
	}
	return memberSettingsGetOutput{Settings: *decoded.Settings}, nil
}

func projectMemberSettingsSetData(data map[string]interface{}) (memberSettingsSetOutput, error) {
	var decoded memberSettingsSetAPIResponse
	if err := decodeMemberAPIData(data, &decoded); err != nil {
		return memberSettingsSetOutput{}, err
	}
	if decoded.Settings == nil {
		return memberSettingsSetOutput{}, memberInvalidResponse("member settings update response is missing settings")
	}
	if decoded.Changes == nil {
		return memberSettingsSetOutput{}, memberInvalidResponse("member settings update response is missing changes")
	}
	if err := validateMemberSettingsResponse(*decoded.Settings); err != nil {
		return memberSettingsSetOutput{}, err
	}
	if err := validateMemberSettingChanges(*decoded.Changes); err != nil {
		return memberSettingsSetOutput{}, err
	}
	return memberSettingsSetOutput{
		Settings: *decoded.Settings, Changes: *decoded.Changes, Changed: decoded.Changed,
	}, nil
}

func projectMemberRecord(raw memberAPIRecord) (memberOutput, error) {
	if !memberStringAllowed(raw.Role, memberRoles) {
		return memberOutput{}, memberInvalidResponse("member response contains an unsupported role")
	}

	typedCount := 0
	for _, value := range []*string{raw.UserOpenID, raw.DepartmentID, raw.ChatID} {
		if value != nil {
			typedCount++
		}
	}
	if typedCount != 1 {
		return memberOutput{}, memberInvalidResponse("member response must contain exactly one typed external ID")
	}

	var memberID, prefix string
	switch raw.MemberType {
	case "user":
		if raw.UserOpenID == nil {
			return memberOutput{}, memberInvalidResponse("member_type does not match the typed external ID")
		}
		memberID, prefix = *raw.UserOpenID, "ou_"
	case "department":
		if raw.DepartmentID == nil {
			return memberOutput{}, memberInvalidResponse("member_type does not match the typed external ID")
		}
		memberID, prefix = *raw.DepartmentID, "od-"
	case "chat":
		if raw.ChatID == nil {
			return memberOutput{}, memberInvalidResponse("member_type does not match the typed external ID")
		}
		memberID, prefix = *raw.ChatID, "oc_"
	default:
		return memberOutput{}, memberInvalidResponse("member response contains an unsupported member_type")
	}
	if !validExternalMemberID(memberID, prefix) {
		return memberOutput{}, memberInvalidResponse("member response contains a malformed external ID; refusing to expose it")
	}

	return memberOutput{
		MemberType: raw.MemberType,
		MemberID:   memberID,
		Name:       raw.Name,
		Role:       raw.Role,
	}, nil
}

func validExternalMemberID(value, prefix string) bool {
	if value != strings.TrimSpace(value) || !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("?#%/\\", r) {
			return false
		}
	}
	return true
}

func validateMemberSettingsResponse(settings memberSettingsResponse) error {
	for _, spec := range memberSettingSpecs {
		value := spec.responseValue(settings)
		if value != nil && !memberStringAllowed(*value, spec.allowed) {
			return memberInvalidResponse("member settings response contains an unsupported %s value", spec.field)
		}
	}
	return nil
}

func validateMemberSettingChanges(changes []memberSettingChangeResponse) error {
	for _, change := range changes {
		spec := memberSettingSpecForField(change.Field)
		if spec == nil {
			return memberInvalidResponse("member settings response contains an unsupported changed field")
		}
		for _, value := range []*string{change.Before, change.After} {
			if value != nil && !memberStringAllowed(*value, spec.allowed) {
				return memberInvalidResponse("member settings response contains an unsupported changed value")
			}
		}
	}
	return nil
}

func memberSettingSpecForField(field string) *memberSettingSpec {
	for index := range memberSettingSpecs {
		if memberSettingSpecs[index].field == field {
			return &memberSettingSpecs[index]
		}
	}
	return nil
}

func memberSettingsSetFlags() []common.Flag {
	flags := make([]common.Flag, 0, len(memberSettingSpecs)+1)
	flags = append(flags, common.Flag{Name: "app-id", Desc: "Miaoda app ID (app_...)"})
	for _, spec := range memberSettingSpecs {
		flags = append(flags, common.Flag{
			Name: spec.flag,
			Desc: spec.description,
			Enum: append([]string(nil), spec.allowed...),
		})
	}
	return flags
}

func memberInvalidResponse(format string, args ...interface{}) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...).
		WithHint("retry the operation; do not use any member ID from this response")
}

func memberDisplayValue(value string) string {
	value = validate.SanitizeForTerminal(value)
	return strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(value))
}
