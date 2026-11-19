// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
)

type formBooleanUpdate struct {
	Enabled *bool `json:"enabled"`
}

type formSubmitPeriodUpdate struct {
	Enabled  *bool   `json:"enabled"`
	StartAt  *string `json:"start_at"`
	EndAt    *string `json:"end_at"`
	Timezone *string `json:"timezone"`
}

type formPerUserSubmitLimitUpdate struct {
	Enabled        *bool   `json:"enabled"`
	FrequencyLimit *int64  `json:"frequency_limit"`
	FrequencyCycle *string `json:"frequency_cycle"`
}

type formGlobalSubmitLimitUpdate struct {
	Enabled *bool  `json:"enabled"`
	Maximum *int64 `json:"maximum"`
}

type formNotificationTarget struct {
	OpenID string `json:"open_id"`
}

type formSubmitNotificationUpdate struct {
	Enabled   *bool                     `json:"enabled"`
	Receivers *[]formNotificationTarget `json:"receivers"`
}

type formScheduledNotificationUpdate struct {
	Enabled    *bool                     `json:"enabled"`
	NotifyTime *string                   `json:"notify_time"`
	RepeatType *string                   `json:"repeat_type"`
	Timezone   *string                   `json:"timezone"`
	Receivers  *[]formNotificationTarget `json:"receivers"`
}

type formResultSegment struct {
	Type   string  `json:"type"`
	Text   *string `json:"text"`
	URL    *string `json:"url"`
	OpenID *string `json:"open_id"`
}

type formSubmitResultPageUpdate struct {
	Enabled     *bool                `json:"enabled"`
	Title       *string              `json:"title"`
	Description *[]formResultSegment `json:"description"`
}

type formRedirectAfterSubmitUpdate struct {
	Enabled *bool   `json:"enabled"`
	URL     *string `json:"url"`
}

type formLotteryAwarderInfo struct {
	Name             string  `json:"name"`
	ContactInfo      string  `json:"contact_info"`
	AwardDeliveryWay *string `json:"award_delivery_way"`
}

type formLotteryAward struct {
	Name     string `json:"name"`
	Quantity int64  `json:"quantity"`
}

func validateFormSubmissionSettingsUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	flags := []string{"submit-period", "per-user-submit-limit", "global-submit-limit", "allow-modify-submission", "ai-voice-input"}
	if err := validateExactlyOneChanged(runtime, flags); err != nil {
		return err
	}
	_, err := buildFormSubmissionSettingsBody(runtime)
	return err
}

func buildFormSubmissionSettingsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	switch {
	case runtime.Changed("submit-period"):
		value, err := decodeStrictJSON[formSubmitPeriodUpdate](runtime, "submit-period")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--submit-period requires enabled")
		}
		if *value.Enabled {
			if value.StartAt == nil || value.EndAt == nil {
				return nil, baseFlagErrorf("--submit-period requires start_at and end_at when enabled")
			}
			start, err := parseRFC3339("--submit-period start_at", *value.StartAt)
			if err != nil {
				return nil, err
			}
			end, err := parseRFC3339("--submit-period end_at", *value.EndAt)
			if err != nil {
				return nil, err
			}
			if end.Before(start) {
				return nil, baseFlagErrorf("--submit-period end_at must not be before start_at")
			}
		}
		return map[string]interface{}{"submit_period": value}, nil
	case runtime.Changed("per-user-submit-limit"):
		value, err := decodeStrictJSON[formPerUserSubmitLimitUpdate](runtime, "per-user-submit-limit")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--per-user-submit-limit requires enabled")
		}
		if *value.Enabled && (value.FrequencyLimit == nil || value.FrequencyCycle == nil) {
			return nil, baseFlagErrorf("--per-user-submit-limit requires frequency_limit and frequency_cycle when enabled")
		}
		if value.FrequencyLimit != nil && *value.FrequencyLimit < 0 {
			return nil, baseFlagErrorf("--per-user-submit-limit frequency_limit must be non-negative")
		}
		if value.FrequencyCycle != nil && !containsString([]string{"total", "day", "week", "month"}, *value.FrequencyCycle) {
			return nil, baseFlagErrorf("--per-user-submit-limit frequency_cycle must be total, day, week, or month")
		}
		return map[string]interface{}{"user_submit_limit": value}, nil
	case runtime.Changed("global-submit-limit"):
		value, err := decodeStrictJSON[formGlobalSubmitLimitUpdate](runtime, "global-submit-limit")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--global-submit-limit requires enabled")
		}
		if *value.Enabled && (value.Maximum == nil || *value.Maximum <= 0) {
			return nil, baseFlagErrorf("--global-submit-limit requires a positive maximum when enabled")
		}
		return map[string]interface{}{"total_submit_limit": value}, nil
	case runtime.Changed("allow-modify-submission"):
		value, err := decodeStrictJSON[formBooleanUpdate](runtime, "allow-modify-submission")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--allow-modify-submission requires enabled")
		}
		return map[string]interface{}{"allow_modify_submission": *value.Enabled}, nil
	case runtime.Changed("ai-voice-input"):
		value, err := decodeStrictJSON[formBooleanUpdate](runtime, "ai-voice-input")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--ai-voice-input requires enabled")
		}
		return map[string]interface{}{"ai_voice_input": value}, nil
	default:
		return nil, baseFlagErrorf("exactly one submission setting flag is required")
	}
}

func validateFormNotificationSettingsUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateExactlyOneChanged(runtime, []string{"locale", "submit-notification", "scheduled-notification"}); err != nil {
		return err
	}
	_, err := buildFormNotificationSettingsBody(runtime)
	return err
}

func buildFormNotificationSettingsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	switch {
	case runtime.Changed("locale"):
		locale := strings.TrimSpace(runtime.Str("locale"))
		if locale == "" {
			return nil, baseFlagErrorf("--locale must not be blank")
		}
		return map[string]interface{}{"locale": locale}, nil
	case runtime.Changed("submit-notification"):
		value, err := decodeStrictJSON[formSubmitNotificationUpdate](runtime, "submit-notification")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--submit-notification requires enabled")
		}
		if *value.Enabled && (value.Receivers == nil || len(*value.Receivers) == 0) {
			return nil, baseFlagErrorf("--submit-notification requires receivers when enabled")
		}
		if value.Receivers != nil {
			if err := validateNotificationTargets("--submit-notification", *value.Receivers); err != nil {
				return nil, err
			}
		}
		return map[string]interface{}{"on_submission": value}, nil
	case runtime.Changed("scheduled-notification"):
		value, err := decodeStrictJSON[formScheduledNotificationUpdate](runtime, "scheduled-notification")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--scheduled-notification requires enabled")
		}
		if *value.Enabled && (value.NotifyTime == nil || value.RepeatType == nil || value.Receivers == nil || len(*value.Receivers) == 0) {
			return nil, baseFlagErrorf("--scheduled-notification requires notify_time, repeat_type, and receivers when enabled")
		}
		if value.NotifyTime != nil {
			if _, err := parseRFC3339("--scheduled-notification notify_time", *value.NotifyTime); err != nil {
				return nil, err
			}
		}
		if value.Receivers != nil {
			if len(*value.Receivers) > 200 {
				return nil, baseFlagErrorf("--scheduled-notification accepts at most 200 receivers")
			}
			if err := validateNotificationTargets("--scheduled-notification", *value.Receivers); err != nil {
				return nil, err
			}
		}
		return map[string]interface{}{"scheduled": value}, nil
	default:
		return nil, baseFlagErrorf("exactly one notification setting flag is required")
	}
}

func validateFormPostSubmitSettingsUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateExactlyOneChanged(runtime, []string{"submit-result-page", "redirect-after-submit"}); err != nil {
		return err
	}
	if runtime.Int("revision") < 0 {
		return baseFlagErrorf("--revision must be non-negative")
	}
	_, err := buildFormPostSubmitSettingsBody(runtime)
	return err
}

func buildFormPostSubmitSettingsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := map[string]interface{}{"revision": runtime.Int("revision")}
	switch {
	case runtime.Changed("submit-result-page"):
		value, err := decodeStrictJSON[formSubmitResultPageUpdate](runtime, "submit-result-page")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--submit-result-page requires enabled")
		}
		if *value.Enabled && (value.Title == nil || value.Description == nil) {
			return nil, baseFlagErrorf("--submit-result-page requires title and description when enabled")
		}
		if value.Title != nil && (len([]rune(*value.Title)) < 1 || len([]rune(*value.Title)) > 12) {
			return nil, baseFlagErrorf("--submit-result-page title must contain 1 to 12 characters")
		}
		if value.Description != nil {
			if err := validateResultSegments(*value.Description); err != nil {
				return nil, err
			}
		}
		body["result_page"] = value
	case runtime.Changed("redirect-after-submit"):
		value, err := decodeStrictJSON[formRedirectAfterSubmitUpdate](runtime, "redirect-after-submit")
		if err != nil {
			return nil, err
		}
		if value.Enabled == nil {
			return nil, baseFlagErrorf("--redirect-after-submit requires enabled")
		}
		if *value.Enabled && (value.URL == nil || strings.TrimSpace(*value.URL) == "") {
			return nil, baseFlagErrorf("--redirect-after-submit requires url when enabled")
		}
		body["redirect"] = value
	default:
		return nil, baseFlagErrorf("exactly one post-submit setting flag is required")
	}
	return body, nil
}

func validateFormLotterySettingsUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	_, err := buildFormLotterySettingsBody(runtime)
	return err
}

func buildFormLotterySettingsBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	action := runtime.Str("action")
	body := map[string]interface{}{"action": action}
	updateFlags := []string{"version", "probability", "awarder-info", "awards"}
	if action != "update" {
		for _, flag := range updateFlags {
			if runtime.Changed(flag) {
				return nil, baseFlagErrorf("--%s is only valid with --action=update", flag)
			}
		}
		return body, nil
	}
	if !runtime.Changed("version") || runtime.Int("version") < 0 {
		return nil, baseFlagErrorf("--version is required and must be non-negative with --action=update")
	}
	lottery := map[string]interface{}{"version": runtime.Int("version")}
	if runtime.Changed("probability") {
		probability := runtime.Int("probability")
		if probability < 0 || probability > 10000 {
			return nil, baseFlagErrorf("--probability must be between 0 and 10000")
		}
		lottery["probability"] = probability
	}
	if runtime.Changed("awarder-info") {
		value, err := decodeStrictJSON[formLotteryAwarderInfo](runtime, "awarder-info")
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.ContactInfo) == "" {
			return nil, baseFlagErrorf("--awarder-info requires name and contact_info")
		}
		lottery["awarder_info"] = value
	}
	if runtime.Changed("awards") {
		value, err := decodeStrictJSON[[]formLotteryAward](runtime, "awards")
		if err != nil {
			return nil, err
		}
		if len(value) > 8 {
			return nil, baseFlagErrorf("--awards accepts at most 8 public awards")
		}
		for _, award := range value {
			if strings.TrimSpace(award.Name) == "" || award.Quantity <= 0 {
				return nil, baseFlagErrorf("--awards contains an invalid name or quantity")
			}
		}
		lottery["awards"] = value
	}
	body["lottery"] = lottery
	return body, nil
}

func decodeStrictJSON[T any](runtime *common.RuntimeContext, flagName string) (T, error) {
	var value T
	raw := strings.TrimSpace(runtime.Str(flagName))
	if raw == "" || raw == "null" {
		return value, baseFlagErrorf("--%s must be non-null JSON", flagName)
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, baseFlagErrorf("--%s contains invalid or unknown JSON fields: %v", flagName, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return value, baseFlagErrorf("--%s must contain exactly one JSON value", flagName)
	}
	return value, nil
}

func validateExactlyOneChanged(runtime *common.RuntimeContext, flags []string) error {
	changed := make([]string, 0, len(flags))
	for _, flag := range flags {
		if runtime.Changed(flag) {
			changed = append(changed, "--"+flag)
		}
	}
	if len(changed) != 1 {
		return baseFlagErrorf("exactly one setting flag is required; received %s", strings.Join(changed, ", "))
	}
	return nil
}

func parseRFC3339(field, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, baseFlagErrorf("%s must be RFC3339 with a timezone: %v", field, err)
	}
	return parsed, nil
}

func validateNotificationTargets(flag string, targets []formNotificationTarget) error {
	for _, target := range targets {
		if !strings.HasPrefix(strings.TrimSpace(target.OpenID), "ou_") {
			return baseFlagErrorf("%s targets must contain user open_id values with the ou_ prefix", flag)
		}
	}
	return nil
}

func validateResultSegments(segments []formResultSegment) error {
	totalLength := 0
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			if segment.Text == nil || segment.URL != nil || segment.OpenID != nil {
				return baseFlagErrorf("--submit-result-page text segments require only text")
			}
			totalLength += len([]rune(*segment.Text))
		case "url":
			if segment.URL == nil || segment.Text != nil || segment.OpenID != nil {
				return baseFlagErrorf("--submit-result-page url segments require only url")
			}
		case "mention":
			if segment.OpenID == nil || !strings.HasPrefix(*segment.OpenID, "ou_") || segment.Text != nil || segment.URL != nil {
				return baseFlagErrorf("--submit-result-page mention segments require only a user open_id")
			}
		default:
			return baseFlagErrorf("--submit-result-page segment type must be text, url, or mention")
		}
	}
	if totalLength > 240 {
		return baseFlagErrorf("--submit-result-page text must not exceed 240 characters")
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
