// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
)

var formConfigCommonFlags = []common.Flag{
	baseTokenFlag(true),
	{Name: "table-id", Desc: "table ID", Required: true},
	{Name: "form-id", Desc: "form ID", Required: true},
}

var BaseFormSubmissionSettingsGet = formConfigGetShortcut(
	"+form-submission-settings-get",
	"Get form submission settings",
	"submission-settings",
)

var BaseFormSubmissionSettingsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-submission-settings-update",
	Description: "Update one form submission setting group",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: appendFormConfigFlags(
		common.Flag{Name: "submission-period-enabled", Type: "bool", Desc: "enable or disable the form submission period"},
		common.Flag{Name: "start-at", Desc: "submission start time in RFC3339 format"},
		common.Flag{Name: "end-at", Desc: "submission end time in RFC3339 format"},
		common.Flag{Name: "timezone", Desc: "IANA timezone, for example Asia/Shanghai"},
		common.Flag{Name: "user-submit-limit-enabled", Type: "bool", Desc: "enable or disable per-user submission limit"},
		common.Flag{Name: "user-submit-limit", Type: "int", Desc: "maximum submissions per user; must be greater than 0"},
		common.Flag{Name: "user-submit-cycle", Desc: "per-user limit cycle", Enum: []string{"total", "day", "week", "month"}},
		common.Flag{Name: "total-submit-limit-enabled", Type: "bool", Desc: "enable or disable total submission limit"},
		common.Flag{Name: "total-submit-maximum", Type: "int", Desc: "maximum total submissions"},
		common.Flag{Name: "allow-modify-submission", Type: "bool", Desc: "allow users to modify submitted records"},
		common.Flag{Name: "ai-voice-input-enabled", Type: "bool", Desc: "enable or disable AI voice input"},
	),
	Tips: []string{
		"Update exactly one top-level group per invocation: submission_period, user_submit_limit, total_submit_limit, allow_modify_submission, or ai_voice_input.",
		"Boolean flags support explicit false, for example --ai-voice-input-enabled=false.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := buildFormSubmissionSettingsBody(runtime)
		return err
	},
	DryRun:  formConfigPatchDryRun("submission-settings", buildFormSubmissionSettingsBody),
	Execute: formConfigPatchExecute("submission-settings", buildFormSubmissionSettingsBody),
}

var BaseFormNotificationsGet = formConfigGetShortcut(
	"+form-notifications-get",
	"Get form notification settings",
	"notifications",
)

var BaseFormNotificationsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-notifications-update",
	Description: "Update form notification settings",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: appendFormConfigFlags(
		common.Flag{Name: "type", Desc: "notification type", Required: true, Enum: []string{"on-submission", "scheduled"}},
		common.Flag{Name: "enabled", Type: "bool", Desc: "enable or disable this notification", Required: true},
		common.Flag{Name: "locale", Desc: "notification locale"},
		common.Flag{Name: "receiver-open-id", Type: "string_array", Desc: "receiver open_id; repeat this flag for multiple receivers"},
		common.Flag{Name: "notify-time", Desc: "scheduled notify time in RFC3339 format"},
		common.Flag{Name: "repeat-type", Desc: "scheduled repeat type", Enum: []string{"no_repeat", "day", "week", "month"}},
		common.Flag{Name: "timezone", Desc: "IANA timezone, for example Asia/Shanghai"},
	),
	Tips: []string{
		"Use --type on-submission or --type scheduled; update only one notification group per invocation.",
		"When enabling either notification type, repeat --receiver-open-id at least once.",
		"Disabling a notification only accepts --type and --enabled=false, plus optional --locale.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := buildFormNotificationsBody(runtime)
		return err
	},
	DryRun:  formConfigPatchDryRun("notifications", buildFormNotificationsBody),
	Execute: formConfigPatchExecute("notifications", buildFormNotificationsBody),
}

var BaseFormSubmitActionsGet = formConfigGetShortcut(
	"+form-submit-actions-get",
	"Get form post-submit actions",
	"submit-actions",
)

var BaseFormSubmitActionsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-submit-actions-update",
	Description: "Update one form post-submit action",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: appendFormConfigFlags(
		common.Flag{Name: "type", Desc: "submit action type", Required: true, Enum: []string{"result-page", "redirect"}},
		common.Flag{Name: "enabled", Type: "bool", Desc: "enable or disable the action", Required: true},
		common.Flag{Name: "revision", Type: "int", Desc: "current action revision; omit to use the latest revision"},
		common.Flag{Name: "title", Desc: "result page title"},
		common.Flag{Name: "description-json", Desc: "result page description JSON array", Input: []string{common.File, common.Stdin}},
		common.Flag{Name: "redirect-url", Desc: "redirect URL after form submit"},
	),
	Tips: []string{
		"Use result-page for the submit result page or redirect for a submit redirect; do not combine fields for both action types.",
		"--description-json supports only text, url, and mention blocks; mention blocks must use open_id.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := buildFormSubmitActionsBody(runtime)
		return err
	},
	DryRun:  formConfigPatchDryRun("submit-actions", buildFormSubmitActionsBody),
	Execute: formConfigPatchExecute("submit-actions", buildFormSubmitActionsBody),
}

var BaseFormLotteryGet = formConfigGetShortcut(
	"+form-lottery-get",
	"Get form lottery settings",
	"lottery",
)

var BaseFormLotteryAction = common.Shortcut{
	Service:     "base",
	Command:     "+form-lottery-action",
	Description: "Run a form lottery action",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: appendFormConfigFlags(
		common.Flag{Name: "action", Desc: "lottery action", Required: true, Enum: []string{"enable", "disable", "update", "relink-winning-table"}},
		common.Flag{Name: "config-json", Desc: "lottery config JSON object; icon_token is not supported", Input: []string{common.File, common.Stdin}},
	),
	Tips: []string{
		"enable and update require --config-json with full lottery settings; update also requires lottery.version.",
		"relink-winning-table accepts optional --config-json without awards; do not pass icon_token.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := buildFormLotteryActionBody(runtime)
		return err
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildFormLotteryActionBody(runtime)
		return common.NewDryRunAPI().
			POST(formConfigDryRunPath("lottery/actions")).
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildFormLotteryActionBody(runtime)
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "POST", formConfigPath(runtime, "lottery", "actions"), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

func formConfigGetShortcut(command, description, segment string) common.Shortcut {
	return common.Shortcut{
		Service:     "base",
		Command:     command,
		Description: description,
		Risk:        "read",
		Scopes:      []string{"base:form:update"},
		AuthTypes:   authTypes(),
		Flags:       formConfigCommonFlags,
		DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return common.NewDryRunAPI().
				GET(formConfigDryRunPath(segment)).
				Set("base_token", runtime.Str("base-token")).
				Set("table_id", runtime.Str("table-id")).
				Set("form_id", runtime.Str("form-id"))
		},
		Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
			data, err := baseV3Call(runtime, "GET", formConfigPath(runtime, segment), nil, nil)
			if err != nil {
				return err
			}
			runtime.Out(data, nil)
			return nil
		},
	}
}

func appendFormConfigFlags(flags ...common.Flag) []common.Flag {
	out := make([]common.Flag, 0, len(formConfigCommonFlags)+len(flags))
	out = append(out, formConfigCommonFlags...)
	out = append(out, flags...)
	return out
}

func formConfigDryRunPath(segment string) string {
	return "/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/" + segment
}

func formConfigPath(runtime *common.RuntimeContext, segments ...string) string {
	parts := []string{"bases", runtime.Str("base-token"), "tables", runtime.Str("table-id"), "forms", runtime.Str("form-id")}
	parts = append(parts, segments...)
	return baseV3Path(parts...)
}

func formConfigPatchDryRun(segment string, build func(*common.RuntimeContext) (map[string]interface{}, error)) func(context.Context, *common.RuntimeContext) *common.DryRunAPI {
	return func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := build(runtime)
		return common.NewDryRunAPI().
			PATCH(formConfigDryRunPath(segment)).
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	}
}

func formConfigPatchExecute(segment string, build func(*common.RuntimeContext) (map[string]interface{}, error)) func(context.Context, *common.RuntimeContext) error {
	return func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := build(runtime)
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "PATCH", formConfigPath(runtime, segment), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	}
}

func buildFormSubmissionSettingsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	groups := changedFormSubmissionGroups(runtime)
	if len(groups) != 1 {
		return nil, baseFlagErrorf("update exactly one form submission settings group; changed groups: %s", strings.Join(groups, ", "))
	}

	body := map[string]interface{}{}
	switch groups[0] {
	case "submission_period":
		if !runtime.Changed("submission-period-enabled") {
			return nil, baseFlagErrorf("--submission-period-enabled is required when updating submission period")
		}
		enabled := runtime.Bool("submission-period-enabled")
		period := map[string]interface{}{"enabled": enabled}
		if enabled {
			if runtime.Str("start-at") == "" {
				return nil, baseFlagErrorf("--start-at is required when --submission-period-enabled=true")
			}
			if runtime.Str("end-at") == "" {
				return nil, baseFlagErrorf("--end-at is required when --submission-period-enabled=true")
			}
			if runtime.Str("timezone") == "" {
				return nil, baseFlagErrorf("--timezone is required when --submission-period-enabled=true")
			}
			if err := validateRFC3339Flag("start-at", runtime.Str("start-at")); err != nil {
				return nil, err
			}
			if err := validateRFC3339Flag("end-at", runtime.Str("end-at")); err != nil {
				return nil, err
			}
			startAt, _ := time.Parse(time.RFC3339, runtime.Str("start-at"))
			endAt, _ := time.Parse(time.RFC3339, runtime.Str("end-at"))
			if !startAt.Before(endAt) {
				return nil, baseFlagErrorf("--start-at must be before --end-at")
			}
			if _, err := time.LoadLocation(runtime.Str("timezone")); err != nil {
				return nil, baseFlagErrorf("--timezone must be a valid IANA timezone: %v", err)
			}
			period["start_at"] = runtime.Str("start-at")
			period["end_at"] = runtime.Str("end-at")
			period["timezone"] = runtime.Str("timezone")
		} else if anyChanged(runtime, "start-at", "end-at", "timezone") {
			return nil, baseFlagErrorf("disabling submission period only accepts --submission-period-enabled=false")
		}
		body["submission_period"] = period
	case "user_submit_limit":
		if !runtime.Changed("user-submit-limit-enabled") {
			return nil, baseFlagErrorf("--user-submit-limit-enabled is required when updating user submit limit")
		}
		enabled := runtime.Bool("user-submit-limit-enabled")
		limit := map[string]interface{}{"enabled": enabled}
		if enabled {
			if runtime.Int("user-submit-limit") <= 0 {
				return nil, baseFlagErrorf("--user-submit-limit must be greater than 0 when --user-submit-limit-enabled=true")
			}
			if runtime.Str("user-submit-cycle") == "" {
				return nil, baseFlagErrorf("--user-submit-cycle is required when --user-submit-limit-enabled=true")
			}
			limit["frequency_limit"] = runtime.Int("user-submit-limit")
			limit["frequency_cycle"] = runtime.Str("user-submit-cycle")
		} else if anyChanged(runtime, "user-submit-limit", "user-submit-cycle") {
			return nil, baseFlagErrorf("disabling user submit limit only accepts --user-submit-limit-enabled=false")
		}
		body["user_submit_limit"] = limit
	case "total_submit_limit":
		if !runtime.Changed("total-submit-limit-enabled") {
			return nil, baseFlagErrorf("--total-submit-limit-enabled is required when updating total submit limit")
		}
		enabled := runtime.Bool("total-submit-limit-enabled")
		limit := map[string]interface{}{"enabled": enabled}
		if enabled {
			if runtime.Int("total-submit-maximum") <= 0 || runtime.Int("total-submit-maximum") > 100000 {
				return nil, baseFlagErrorf("--total-submit-maximum must be between 1 and 100000 when --total-submit-limit-enabled=true")
			}
			limit["maximum"] = runtime.Int("total-submit-maximum")
		} else if runtime.Changed("total-submit-maximum") {
			return nil, baseFlagErrorf("disabling total submit limit only accepts --total-submit-limit-enabled=false")
		}
		body["total_submit_limit"] = limit
	case "allow_modify_submission":
		body["allow_modify_submission"] = runtime.Bool("allow-modify-submission")
	case "ai_voice_input":
		body["ai_voice_input"] = map[string]interface{}{"enabled": runtime.Bool("ai-voice-input-enabled")}
	}
	return body, nil
}

func changedFormSubmissionGroups(runtime *common.RuntimeContext) []string {
	groups := []string{}
	if anyChanged(runtime, "submission-period-enabled", "start-at", "end-at", "timezone") {
		groups = append(groups, "submission_period")
	}
	if anyChanged(runtime, "user-submit-limit-enabled", "user-submit-limit", "user-submit-cycle") {
		groups = append(groups, "user_submit_limit")
	}
	if anyChanged(runtime, "total-submit-limit-enabled", "total-submit-maximum") {
		groups = append(groups, "total_submit_limit")
	}
	if runtime.Changed("allow-modify-submission") {
		groups = append(groups, "allow_modify_submission")
	}
	if runtime.Changed("ai-voice-input-enabled") {
		groups = append(groups, "ai_voice_input")
	}
	return groups
}

func buildFormNotificationsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	notificationType := runtime.Str("type")
	switch notificationType {
	case "on-submission", "scheduled":
	default:
		return nil, baseFlagErrorf("--type must be on-submission or scheduled")
	}

	enabled := runtime.Bool("enabled")
	body := map[string]interface{}{}
	if runtime.Changed("locale") {
		body["locale"] = runtime.Str("locale")
	}

	group := map[string]interface{}{"enabled": enabled}
	receivers, err := formNotificationReceivers(runtime.StrArray("receiver-open-id"))
	if err != nil {
		return nil, err
	}
	if enabled && len(receivers) == 0 {
		return nil, baseFlagErrorf("at least one --receiver-open-id is required when notification is enabled")
	}
	if notificationType == "scheduled" {
		if enabled {
			if runtime.Str("notify-time") == "" {
				return nil, baseFlagErrorf("--notify-time is required when scheduled notification is enabled")
			}
			if runtime.Str("repeat-type") == "" {
				return nil, baseFlagErrorf("--repeat-type is required when scheduled notification is enabled")
			}
			if runtime.Str("timezone") == "" {
				return nil, baseFlagErrorf("--timezone is required when scheduled notification is enabled")
			}
			if err := validateRFC3339Flag("notify-time", runtime.Str("notify-time")); err != nil {
				return nil, err
			}
			if _, err := time.LoadLocation(runtime.Str("timezone")); err != nil {
				return nil, baseFlagErrorf("--timezone must be a valid IANA timezone: %v", err)
			}
			group["receivers"] = receivers
			group["notify_time"] = runtime.Str("notify-time")
			group["repeat_type"] = runtime.Str("repeat-type")
			group["timezone"] = runtime.Str("timezone")
		} else if anyChanged(runtime, "receiver-open-id", "notify-time", "repeat-type", "timezone") {
			return nil, baseFlagErrorf("disabling scheduled notification only accepts --type, --enabled=false, and optional --locale")
		}
		body["scheduled"] = group
		return body, nil
	}

	if !enabled && anyChanged(runtime, "receiver-open-id", "notify-time", "repeat-type", "timezone") {
		return nil, baseFlagErrorf("disabling on-submission notification only accepts --type, --enabled=false, and optional --locale")
	}
	if enabled {
		group["receivers"] = receivers
	}
	body["on_submission"] = group
	return body, nil
}

func buildFormSubmitActionsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	enabled := runtime.Bool("enabled")
	body := map[string]interface{}{}
	if runtime.Changed("revision") {
		body["revision"] = runtime.Int("revision")
	}
	group := map[string]interface{}{"enabled": enabled}

	switch runtime.Str("type") {
	case "result-page":
		if runtime.Changed("redirect-url") {
			return nil, baseFlagErrorf("--redirect-url cannot be used with --type result-page")
		}
		if enabled {
			if runtime.Str("title") == "" {
				return nil, baseFlagErrorf("--title is required when result page is enabled")
			}
			if runtime.Str("description-json") == "" {
				return nil, baseFlagErrorf("--description-json is required when result page is enabled")
			}
			description, err := parseJSONArrayFlag("description-json", runtime.Str("description-json"))
			if err != nil {
				return nil, err
			}
			if err := validateResultPageDescription(description); err != nil {
				return nil, err
			}
			group["title"] = runtime.Str("title")
			group["description"] = description
		} else if runtime.Changed("title") || runtime.Changed("description-json") {
			return nil, baseFlagErrorf("disabling result page must not include --title or --description-json")
		}
		body["result_page"] = group
		return body, nil
	case "redirect":
		if runtime.Changed("title") || runtime.Changed("description-json") {
			return nil, baseFlagErrorf("--title and --description-json cannot be used with --type redirect")
		}
		if enabled {
			if runtime.Str("redirect-url") == "" {
				return nil, baseFlagErrorf("--redirect-url is required when redirect is enabled")
			}
			if !isHTTPSFormConfigURL(runtime.Str("redirect-url")) {
				return nil, baseFlagErrorf("--redirect-url must be a valid HTTPS URL")
			}
			group["url"] = runtime.Str("redirect-url")
		} else if runtime.Changed("redirect-url") {
			return nil, baseFlagErrorf("disabling redirect must not include --redirect-url")
		}
		body["redirect"] = group
		return body, nil
	default:
		return nil, baseFlagErrorf("--type must be result-page or redirect")
	}
}

func buildFormLotteryActionBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	action := runtime.Str("action")
	wireAction := action
	if action == "relink-winning-table" {
		wireAction = "relink_winning_table"
	}
	body := map[string]interface{}{"action": wireAction}
	config := runtime.Str("config-json")
	if action == "disable" && runtime.Changed("config-json") {
		return nil, baseFlagErrorf("--config-json is not accepted for disable action")
	}
	if action == "enable" || action == "update" {
		if config == "" {
			return nil, baseFlagErrorf("--config-json is required for %s action", action)
		}
	}
	if config != "" {
		lottery, err := parseFormLotteryPayload(config)
		if err != nil {
			return nil, err
		}
		if action == "relink-winning-table" && lottery.AwardsSet {
			return nil, baseFlagErrorf("--config-json must not contain awards for relink-winning-table action")
		}
		if action == "update" {
			if lottery.Version == nil {
				return nil, baseFlagErrorf("--config-json.version is required for update action")
			}
		}
		if action == "enable" || action == "update" {
			if err := validateLotteryConfig(lottery, action == "update"); err != nil {
				return nil, err
			}
		}
		body["lottery"] = lottery
		return body, nil
	}
	return body, nil
}

func formNotificationReceivers(openIDs []string) ([]interface{}, error) {
	receivers := make([]interface{}, 0, len(openIDs))
	for _, openID := range openIDs {
		openID = strings.TrimSpace(openID)
		if openID == "" {
			return nil, baseFlagErrorf("--receiver-open-id must be non-empty")
		}
		receivers = append(receivers, map[string]interface{}{"open_id": openID})
	}
	return receivers, nil
}

type formLotteryPayload struct {
	Version     *int64                     `json:"version,omitempty"`
	Probability *int64                     `json:"probability,omitempty"`
	AwarderInfo *formLotteryAwarderPayload `json:"awarder_info,omitempty"`
	Awards      *[]formLotteryAwardPayload `json:"awards,omitempty"`
	AwardsSet   bool                       `json:"-"`
}

type formLotteryAwarderPayload struct {
	Name             string  `json:"name"`
	ContactInfo      string  `json:"contact_info"`
	AwardDeliveryWay *string `json:"award_delivery_way,omitempty"`
}

type formLotteryAwardPayload struct {
	Name     string `json:"name"`
	Quantity int64  `json:"quantity"`
}

func parseFormLotteryPayload(value string) (*formLotteryPayload, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var lottery *formLotteryPayload
	if err := decoder.Decode(&lottery); err != nil || lottery == nil {
		return nil, baseFlagErrorf("--config-json must be a valid lottery JSON object without unsupported fields: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, baseFlagErrorf("--config-json must contain exactly one JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &fields); err != nil {
		return nil, baseFlagErrorf("--config-json must be a valid lottery JSON object: %v", err)
	}
	for field := range fields {
		if strings.EqualFold(field, "awards") {
			lottery.AwardsSet = true
			break
		}
	}
	return lottery, nil
}

func validateLotteryConfig(lottery *formLotteryPayload, requireVersion bool) error {
	if requireVersion {
		if lottery.Version == nil || *lottery.Version < 0 {
			return baseFlagErrorf("--config-json.version must be a non-negative integer for update action")
		}
	}
	if lottery.Probability == nil || *lottery.Probability < 0 || *lottery.Probability > 10000 {
		return baseFlagErrorf("--config-json.probability must be an integer between 0 and 10000")
	}
	awarder := lottery.AwarderInfo
	if awarder == nil || strings.TrimSpace(awarder.Name) == "" || strings.TrimSpace(awarder.ContactInfo) == "" {
		return baseFlagErrorf("--config-json.awarder_info requires non-empty name and contact_info")
	}
	if awarder.AwardDeliveryWay != nil && strings.TrimSpace(*awarder.AwardDeliveryWay) == "" {
		return baseFlagErrorf("--config-json.awarder_info.award_delivery_way must be a non-empty string when provided")
	}
	if lottery.Awards == nil || len(*lottery.Awards) == 0 || len(*lottery.Awards) > 7 {
		return baseFlagErrorf("--config-json.awards must contain between 1 and 7 awards")
	}
	awards := *lottery.Awards
	awardNames := make(map[string]bool, len(awards))
	for index, award := range awards {
		name := strings.TrimSpace(award.Name)
		if name == "" {
			return baseFlagErrorf("--config-json.awards[%d].name must be a non-empty string", index)
		}
		if awardNames[name] {
			return baseFlagErrorf("--config-json award names must be unique")
		}
		awardNames[name] = true
		if award.Quantity <= 0 {
			return baseFlagErrorf("--config-json.awards[%d].quantity must be a positive integer", index)
		}
	}
	return nil
}

func nonEmptyLotteryString(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func validateRFC3339Flag(name, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return baseFlagErrorf("--%s must be RFC3339: %v", name, err)
	}
	return nil
}

func parseJSONArrayFlag(name, value string) ([]interface{}, error) {
	var body []interface{}
	if err := json.Unmarshal([]byte(value), &body); err != nil {
		return nil, baseFlagErrorf("--%s must be valid JSON array: %v", name, err)
	}
	if body == nil {
		return nil, baseFlagErrorf("--%s must be valid JSON array", name)
	}
	return body, nil
}

func validateResultPageDescription(items []interface{}) error {
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return baseFlagErrorf("--description-json items must be JSON objects")
		}
		typ, _ := obj["type"].(string)
		switch typ {
		case "text":
			if nonEmptyLotteryString(obj["text"]) == "" || obj["url"] != nil || obj["open_id"] != nil {
				return baseFlagErrorf("--description-json text items only accept non-empty text")
			}
		case "url":
			urlValue, _ := obj["url"].(string)
			if nonEmptyLotteryString(obj["text"]) == "" || !isHTTPSFormConfigURL(urlValue) || obj["open_id"] != nil {
				return baseFlagErrorf("--description-json url items require non-empty text and an HTTPS url")
			}
		case "mention":
			if nonEmptyLotteryString(obj["open_id"]) == "" || obj["url"] != nil {
				return baseFlagErrorf("--description-json mention items require a non-empty open_id")
			}
		default:
			return baseFlagErrorf("--description-json item type must be text, url, or mention")
		}
	}
	return nil
}

func isHTTPSFormConfigURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func anyChanged(runtime *common.RuntimeContext, names ...string) bool {
	for _, name := range names {
		if runtime.Changed(name) {
			return true
		}
	}
	return false
}
