// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const appFeedCardCreatePath = "/open-apis/im/v2/app_feed_card"
const appFeedCardBatchPath = "/open-apis/im/v2/app_feed_card/batch"

var ImAppFeedCardCreate = common.Shortcut{
	Service:     "im",
	Command:     "+app-feed-card-create",
	Description: "Create an app feed card for users; bot only; supports title, preview, link, status label, buttons, notification settings, and raw card JSON",
	Risk:        "write",
	Scopes:      []string{"im:app_feed_card:write"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-ids", Desc: "recipient user IDs, comma-separated; IDs must match --user-id-type", Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "recipient ID type", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "card-json", Desc: "app_feed_card JSON object; scalar card flags override matching fields", Input: []string{common.File, common.Stdin}},
		{Name: "biz-id", Desc: "custom business ID; if omitted, API returns a generated biz_id"},
		{Name: "title", Desc: "card title"},
		{Name: "avatar-key", Desc: "card avatar key"},
		{Name: "preview", Desc: "card preview text"},
		{Name: "link", Desc: "card redirect link; HTTPS or applink, required unless provided in --card-json"},
		{Name: "status-label-text", Desc: "status label text"},
		{Name: "status-label-type", Default: "primary", Desc: "status label type", Enum: []string{"primary", "secondary", "success", "danger"}},
		{Name: "time-sensitive", Type: "bool", Desc: "temporarily pin the card at the top of the feed"},
		{Name: "buttons-json", Desc: `buttons JSON; accepts {"buttons":[...]} or an array of button objects`, Input: []string{common.File, common.Stdin}},
		{Name: "button-text", Desc: "single convenience button text; use --buttons-json for multiple buttons"},
		{Name: "button-url", Desc: "single convenience button HTTPS URL"},
		{Name: "button-action-type", Default: "url_page", Desc: "single convenience button action type", Enum: []string{"url_page", "webhook"}},
		{Name: "button-type", Default: "default", Desc: "single convenience button style", Enum: []string{"default", "primary", "success"}},
		{Name: "button-action-map-json", Desc: "single convenience button action_map JSON object", Input: []string{common.File, common.Stdin}},
		{Name: "close-notify", Type: "bool", Desc: "disable normal notification for this feed card"},
		{Name: "with-custom-sound", Type: "bool", Desc: "play custom sound on mobile devices"},
		{Name: "custom-sound-text", Desc: "custom mobile sound notification text"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildAppFeedCardCreateBody(runtime)
		return common.NewDryRunAPI().
			POST(appFeedCardCreatePath).
			Params(map[string]interface{}{"user_id_type": appFeedCardUserIDType(runtime)}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildAppFeedCardCreateBody(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildAppFeedCardCreateBody(runtime)
		if err != nil {
			return err
		}

		resData, err := runtime.DoAPIJSON(http.MethodPost, appFeedCardCreatePath,
			larkcore.QueryParams{"user_id_type": []string{appFeedCardUserIDType(runtime)}},
			body,
		)
		if err != nil {
			return err
		}

		userIDs, _ := body["user_ids"].([]string)
		failedCards := normalizeAppFeedFailedCards(resData["failed_cards"])
		out := map[string]interface{}{
			"biz_id":               resData["biz_id"],
			"requested_user_count": len(userIDs),
			"failed_count":         len(failedCards),
			"failed_cards":         failedCards,
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			output.PrintTable(w, []map[string]interface{}{{
				"biz_id":               out["biz_id"],
				"requested_user_count": out["requested_user_count"],
				"failed_count":         out["failed_count"],
			}})
			if len(failedCards) > 0 {
				fmt.Fprintln(w, "\nFailed cards:")
				output.PrintTable(w, failedCards)
			}
		})
		return nil
	},
}

var ImAppFeedCardUpdate = common.Shortcut{
	Service:     "im",
	Command:     "+app-feed-card-update",
	Description: "Update app feed cards for users; bot only; supports single-card flags and raw feed_cards JSON",
	Risk:        "write",
	Scopes:      []string{"im:app_feed_card:write"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-ids", Desc: "recipient user IDs, comma-separated; used with --biz-id to update the same card for each user"},
		{Name: "user-id-type", Default: "open_id", Desc: "recipient ID type", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "feed-cards-json", Desc: `raw feed_cards JSON; accepts {"feed_cards":[...]} or an array`, Input: []string{common.File, common.Stdin}},
		{Name: "update-fields", Desc: "fields to update, comma-separated; accepts names or API enum values: title=1, avatar_key=2, preview=3, status_label=10, buttons=11, link=12, time_sensitive=13, notify=103; defaults to card fields supplied by flags"},
		{Name: "card-json", Desc: "app_feed_card JSON object; scalar card flags override matching fields", Input: []string{common.File, common.Stdin}},
		{Name: "biz-id", Desc: "business ID of the app feed card to update"},
		{Name: "title", Desc: "card title"},
		{Name: "avatar-key", Desc: "card avatar key"},
		{Name: "preview", Desc: "card preview text"},
		{Name: "link", Desc: "card redirect link; HTTPS or applink"},
		{Name: "status-label-text", Desc: "status label text"},
		{Name: "status-label-type", Default: "primary", Desc: "status label type", Enum: []string{"primary", "secondary", "success", "danger"}},
		{Name: "time-sensitive", Type: "bool", Desc: "temporarily pin the card at the top of the feed"},
		{Name: "buttons-json", Desc: `buttons JSON; accepts {"buttons":[...]} or an array of button objects`, Input: []string{common.File, common.Stdin}},
		{Name: "button-text", Desc: "single convenience button text; use --buttons-json for multiple buttons"},
		{Name: "button-url", Desc: "single convenience button HTTPS URL"},
		{Name: "button-action-type", Default: "url_page", Desc: "single convenience button action type", Enum: []string{"url_page", "webhook"}},
		{Name: "button-type", Default: "default", Desc: "single convenience button style", Enum: []string{"default", "primary", "success"}},
		{Name: "button-action-map-json", Desc: "single convenience button action_map JSON object", Input: []string{common.File, common.Stdin}},
		{Name: "close-notify", Type: "bool", Desc: "disable normal notification for this feed card"},
		{Name: "with-custom-sound", Type: "bool", Desc: "play custom sound on mobile devices"},
		{Name: "custom-sound-text", Desc: "custom mobile sound notification text"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildAppFeedCardUpdateBody(runtime)
		return common.NewDryRunAPI().
			PUT(appFeedCardBatchPath).
			Params(map[string]interface{}{"user_id_type": appFeedCardUserIDType(runtime)}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildAppFeedCardUpdateBody(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildAppFeedCardUpdateBody(runtime)
		if err != nil {
			return err
		}
		resData, err := runtime.DoAPIJSON(http.MethodPut, appFeedCardBatchPath,
			larkcore.QueryParams{"user_id_type": []string{appFeedCardUserIDType(runtime)}},
			body,
		)
		if err != nil {
			return err
		}
		return outputAppFeedCardBatchResult(runtime, body, resData)
	},
}

var ImAppFeedCardDelete = common.Shortcut{
	Service:     "im",
	Command:     "+app-feed-card-delete",
	Description: "Delete app feed cards for users by biz_id; bot only; supports raw feed_cards JSON",
	Risk:        "write",
	Scopes:      []string{"im:app_feed_card:write"},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-ids", Desc: "recipient user IDs, comma-separated; used with --biz-id to delete the same card for each user"},
		{Name: "user-id-type", Default: "open_id", Desc: "recipient ID type", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "feed-cards-json", Desc: `raw feed_cards JSON; accepts {"feed_cards":[...]} or an array`, Input: []string{common.File, common.Stdin}},
		{Name: "biz-id", Desc: "business ID of the app feed card to delete"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildAppFeedCardDeleteBody(runtime)
		return common.NewDryRunAPI().
			DELETE(appFeedCardBatchPath).
			Params(map[string]interface{}{"user_id_type": appFeedCardUserIDType(runtime)}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildAppFeedCardDeleteBody(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildAppFeedCardDeleteBody(runtime)
		if err != nil {
			return err
		}
		resData, err := runtime.DoAPIJSON(http.MethodDelete, appFeedCardBatchPath,
			larkcore.QueryParams{"user_id_type": []string{appFeedCardUserIDType(runtime)}},
			body,
		)
		if err != nil {
			return err
		}
		return outputAppFeedCardBatchResult(runtime, body, resData)
	},
}

func appFeedCardUserIDType(runtime *common.RuntimeContext) string {
	if v := runtime.Str("user-id-type"); v != "" {
		return v
	}
	return "open_id"
}

func buildAppFeedCardCreateBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	userIDs := common.SplitCSV(runtime.Str("user-ids"))
	if err := validateAppFeedUserIDs(userIDs, appFeedCardUserIDType(runtime), "--user-ids"); err != nil {
		return nil, err
	}

	card, err := buildAppFeedCardObject(runtime)
	if err != nil {
		return nil, err
	}
	if err := validateAppFeedCardObject(card); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"app_feed_card": card,
		"user_ids":      userIDs,
	}, nil
}

func buildAppFeedCardUpdateBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	if raw := strings.TrimSpace(runtime.Str("feed-cards-json")); raw != "" {
		feedCards, err := parseAppFeedCardsJSON(raw, "--feed-cards-json")
		if err != nil {
			return nil, err
		}
		if err := validateAppFeedCardUpdateItems(feedCards, appFeedCardUserIDType(runtime)); err != nil {
			return nil, err
		}
		return map[string]interface{}{"feed_cards": feedCards}, nil
	}

	userIDs := common.SplitCSV(runtime.Str("user-ids"))
	if err := validateAppFeedUserIDs(userIDs, appFeedCardUserIDType(runtime), "--user-ids"); err != nil {
		return nil, err
	}

	card, err := buildAppFeedCardObject(runtime)
	if err != nil {
		return nil, err
	}
	if err := validateAppFeedCardUpdateObject(card); err != nil {
		return nil, err
	}

	updateFields, err := normalizeAppFeedCardUpdateFields(common.SplitCSV(runtime.Str("update-fields")))
	if err != nil {
		return nil, err
	}
	if len(updateFields) == 0 {
		updateFields = deriveAppFeedCardUpdateFields(card)
	}
	if len(updateFields) == 0 {
		return nil, output.ErrValidation("--update-fields is required when no updatable card fields are supplied")
	}

	feedCards := make([]interface{}, 0, len(userIDs))
	for _, userID := range userIDs {
		feedCards = append(feedCards, map[string]interface{}{
			"app_feed_card": cloneAppFeedValue(card),
			"user_id":       userID,
			"update_fields": append([]string(nil), updateFields...),
		})
	}
	return map[string]interface{}{"feed_cards": feedCards}, nil
}

func buildAppFeedCardDeleteBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	if raw := strings.TrimSpace(runtime.Str("feed-cards-json")); raw != "" {
		feedCards, err := parseAppFeedCardsJSON(raw, "--feed-cards-json")
		if err != nil {
			return nil, err
		}
		if err := validateAppFeedCardDeleteItems(feedCards, appFeedCardUserIDType(runtime)); err != nil {
			return nil, err
		}
		return map[string]interface{}{"feed_cards": feedCards}, nil
	}

	userIDs := common.SplitCSV(runtime.Str("user-ids"))
	if err := validateAppFeedUserIDs(userIDs, appFeedCardUserIDType(runtime), "--user-ids"); err != nil {
		return nil, err
	}
	bizID := strings.TrimSpace(runtime.Str("biz-id"))
	if bizID == "" {
		return nil, output.ErrValidation("--biz-id is required unless --feed-cards-json is provided")
	}

	feedCards := make([]interface{}, 0, len(userIDs))
	for _, userID := range userIDs {
		feedCards = append(feedCards, map[string]interface{}{
			"biz_id":  bizID,
			"user_id": userID,
		})
	}
	return map[string]interface{}{"feed_cards": feedCards}, nil
}

func buildAppFeedCardObject(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	card := map[string]interface{}{}
	if raw := strings.TrimSpace(runtime.Str("card-json")); raw != "" {
		parsed, err := parseAppFeedJSONObject(raw, "--card-json")
		if err != nil {
			return nil, err
		}
		if nested, ok := parsed["app_feed_card"].(map[string]interface{}); ok {
			parsed = nested
		}
		card = parsed
	}
	normalizeAppFeedLink(card)

	setStringField(card, "biz-id", "biz_id", runtime)
	setStringField(card, "title", "title", runtime)
	setStringField(card, "avatar-key", "avatar_key", runtime)
	setStringField(card, "preview", "preview", runtime)
	if link := strings.TrimSpace(runtime.Str("link")); link != "" {
		card["link"] = map[string]interface{}{"link": link}
	}
	if flagChanged(runtime, "time-sensitive") {
		card["time_sensitive"] = runtime.Bool("time-sensitive")
	}

	if err := setAppFeedStatusLabel(card, runtime); err != nil {
		return nil, err
	}
	if err := setAppFeedButtons(card, runtime); err != nil {
		return nil, err
	}
	if err := setAppFeedNotify(card, runtime); err != nil {
		return nil, err
	}
	return card, nil
}

func setStringField(card map[string]interface{}, flagName, fieldName string, runtime *common.RuntimeContext) {
	if v := runtime.Str(flagName); v != "" {
		card[fieldName] = v
	}
}

func setAppFeedStatusLabel(card map[string]interface{}, runtime *common.RuntimeContext) error {
	text := runtime.Str("status-label-text")
	if text == "" {
		if flagChanged(runtime, "status-label-type") {
			return output.ErrValidation("--status-label-type requires --status-label-text")
		}
		return nil
	}
	status := objectField(card, "status_label")
	status["text"] = text
	status["type"] = runtime.Str("status-label-type")
	if status["type"] == "" {
		status["type"] = "primary"
	}
	return nil
}

func setAppFeedButtons(card map[string]interface{}, runtime *common.RuntimeContext) error {
	if raw := strings.TrimSpace(runtime.Str("buttons-json")); raw != "" {
		if simpleButtonFlagsSet(runtime) {
			return output.ErrValidation("--buttons-json cannot be combined with --button-text, --button-url, --button-action-type, --button-type, or --button-action-map-json")
		}
		buttons, err := parseAppFeedButtonsJSON(raw)
		if err != nil {
			return err
		}
		card["buttons"] = buttons
		return nil
	}
	if !simpleButtonFlagsSet(runtime) {
		return nil
	}

	text := runtime.Str("button-text")
	actionType := runtime.Str("button-action-type")
	if actionType == "" {
		actionType = "url_page"
	}
	if text == "" {
		return output.ErrValidation("--button-text is required when setting a convenience button")
	}
	button := map[string]interface{}{
		"action_type": actionType,
		"text":        map[string]interface{}{"text": text},
	}
	if buttonType := runtime.Str("button-type"); buttonType != "" {
		button["button_type"] = buttonType
	}
	if buttonURL := strings.TrimSpace(runtime.Str("button-url")); buttonURL != "" {
		button["multi_url"] = map[string]interface{}{"url": buttonURL}
	}
	if rawActionMap := strings.TrimSpace(runtime.Str("button-action-map-json")); rawActionMap != "" {
		actionMap, err := parseAppFeedJSONObject(rawActionMap, "--button-action-map-json")
		if err != nil {
			return err
		}
		button["action_map"] = actionMap
	}
	card["buttons"] = map[string]interface{}{"buttons": []interface{}{button}}
	return nil
}

func setAppFeedNotify(card map[string]interface{}, runtime *common.RuntimeContext) error {
	if !flagChanged(runtime, "close-notify") && !flagChanged(runtime, "with-custom-sound") && runtime.Str("custom-sound-text") == "" {
		return nil
	}
	notify := objectField(card, "notify")
	if flagChanged(runtime, "close-notify") {
		notify["close_notify"] = runtime.Bool("close-notify")
	}
	if flagChanged(runtime, "with-custom-sound") {
		notify["with_custom_sound"] = runtime.Bool("with-custom-sound")
	}
	if text := runtime.Str("custom-sound-text"); text != "" {
		notify["custom_sound_text"] = text
	}
	return nil
}

func simpleButtonFlagsSet(runtime *common.RuntimeContext) bool {
	return runtime.Str("button-text") != "" ||
		runtime.Str("button-url") != "" ||
		flagChanged(runtime, "button-action-type") ||
		flagChanged(runtime, "button-type") ||
		strings.TrimSpace(runtime.Str("button-action-map-json")) != ""
}

func objectField(parent map[string]interface{}, key string) map[string]interface{} {
	if existing, ok := parent[key].(map[string]interface{}); ok {
		return existing
	}
	next := map[string]interface{}{}
	parent[key] = next
	return next
}

func normalizeAppFeedLink(card map[string]interface{}) {
	if raw, ok := card["link"].(string); ok {
		card["link"] = map[string]interface{}{"link": raw}
	}
}

func parseAppFeedJSONObject(raw, flagName string) (map[string]interface{}, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, output.ErrValidation("%s invalid JSON object: %s", flagName, err)
	}
	if parsed == nil {
		return nil, output.ErrValidation("%s must be a JSON object", flagName)
	}
	return parsed, nil
}

func parseAppFeedButtonsJSON(raw string) (map[string]interface{}, error) {
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, output.ErrValidation("--buttons-json invalid JSON: %s", err)
	}
	switch v := parsed.(type) {
	case map[string]interface{}:
		if _, ok := v["buttons"]; ok {
			return v, nil
		}
		if _, ok := v["text"]; ok {
			return map[string]interface{}{"buttons": []interface{}{v}}, nil
		}
		if _, ok := v["action_type"]; ok {
			return map[string]interface{}{"buttons": []interface{}{v}}, nil
		}
		return nil, output.ErrValidation(`--buttons-json object must contain "buttons" or be a single button object`)
	case []interface{}:
		return map[string]interface{}{"buttons": v}, nil
	default:
		return nil, output.ErrValidation("--buttons-json must be a JSON object or array")
	}
}

func parseAppFeedCardsJSON(raw, flagName string) ([]interface{}, error) {
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, output.ErrValidation("%s invalid JSON: %s", flagName, err)
	}
	switch v := parsed.(type) {
	case map[string]interface{}:
		if rawFeedCards, ok := v["feed_cards"]; ok {
			feedCards, ok := rawFeedCards.([]interface{})
			if !ok {
				return nil, output.ErrValidation(`%s.feed_cards must be a JSON array`, flagName)
			}
			if len(feedCards) == 0 {
				return nil, output.ErrValidation("%s.feed_cards must contain at least one card", flagName)
			}
			return feedCards, nil
		}
		return []interface{}{v}, nil
	case []interface{}:
		if len(v) == 0 {
			return nil, output.ErrValidation("%s must contain at least one card", flagName)
		}
		return v, nil
	default:
		return nil, output.ErrValidation("%s must be a JSON object or array", flagName)
	}
}

func validateAppFeedCardObject(card map[string]interface{}) error {
	if card == nil {
		return output.ErrValidation("app_feed_card cannot be empty")
	}
	normalizeAppFeedLink(card)
	link := appFeedCardLinkValue(card)
	if link == "" {
		return output.ErrValidation("--link is required unless --card-json contains app_feed_card.link.link")
	}
	if err := validateAppFeedURL("--link", link, true); err != nil {
		return err
	}
	if err := validateAppFeedStatusLabel(card); err != nil {
		return err
	}
	if err := validateAppFeedButtons(card); err != nil {
		return err
	}
	return nil
}

func validateAppFeedCardUpdateObject(card map[string]interface{}) error {
	if len(card) == 0 {
		return output.ErrValidation("app_feed_card cannot be empty")
	}
	normalizeAppFeedLink(card)
	if strings.TrimSpace(stringField(card, "biz_id")) == "" {
		return output.ErrValidation("--biz-id is required unless --card-json contains app_feed_card.biz_id")
	}
	if link := appFeedCardLinkValue(card); link != "" {
		if err := validateAppFeedURL("--link", link, true); err != nil {
			return err
		}
	}
	if err := validateAppFeedStatusLabel(card); err != nil {
		return err
	}
	if err := validateAppFeedButtons(card); err != nil {
		return err
	}
	return nil
}

func validateAppFeedUserIDs(userIDs []string, userIDType, flagName string) error {
	if len(userIDs) == 0 {
		return output.ErrValidation("%s is required and must contain at least one recipient ID", flagName)
	}
	if userIDType == "open_id" {
		for _, id := range userIDs {
			if _, err := common.ValidateUserID(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAppFeedCardDeleteItems(feedCards []interface{}, userIDType string) error {
	for i, raw := range feedCards {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return output.ErrValidation("feed_cards[%d] must be a JSON object", i)
		}
		if strings.TrimSpace(stringField(item, "biz_id")) == "" {
			return output.ErrValidation("feed_cards[%d].biz_id is required", i)
		}
		userID := strings.TrimSpace(stringField(item, "user_id"))
		if userID == "" {
			return output.ErrValidation("feed_cards[%d].user_id is required", i)
		}
		if err := validateAppFeedUserIDs([]string{userID}, userIDType, fmt.Sprintf("feed_cards[%d].user_id", i)); err != nil {
			return err
		}
	}
	return nil
}

func validateAppFeedCardUpdateItems(feedCards []interface{}, userIDType string) error {
	for i, raw := range feedCards {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return output.ErrValidation("feed_cards[%d] must be a JSON object", i)
		}
		userID := strings.TrimSpace(stringField(item, "user_id"))
		if userID == "" {
			return output.ErrValidation("feed_cards[%d].user_id is required", i)
		}
		if err := validateAppFeedUserIDs([]string{userID}, userIDType, fmt.Sprintf("feed_cards[%d].user_id", i)); err != nil {
			return err
		}
		card, ok := item["app_feed_card"].(map[string]interface{})
		if !ok {
			return output.ErrValidation("feed_cards[%d].app_feed_card must be a JSON object", i)
		}
		if err := validateAppFeedCardUpdateObject(card); err != nil {
			return err
		}
		updateFields, err := normalizeAppFeedCardUpdateFields(stringSliceField(item, "update_fields"))
		if err != nil {
			return err
		}
		if len(updateFields) == 0 {
			return output.ErrValidation("feed_cards[%d].update_fields must contain at least one field", i)
		}
		item["update_fields"] = updateFields
	}
	return nil
}

func appFeedCardLinkValue(card map[string]interface{}) string {
	linkObj, _ := card["link"].(map[string]interface{})
	if linkObj == nil {
		return ""
	}
	link, _ := linkObj["link"].(string)
	return strings.TrimSpace(link)
}

func validateAppFeedStatusLabel(card map[string]interface{}) error {
	status, ok := card["status_label"].(map[string]interface{})
	if !ok || len(status) == 0 {
		return nil
	}
	text, _ := status["text"].(string)
	labelType, _ := status["type"].(string)
	if strings.TrimSpace(text) == "" {
		return output.ErrValidation("status_label.text is required when status_label is set")
	}
	if !oneOf(labelType, "primary", "secondary", "success", "danger") {
		return output.ErrValidation("status_label.type must be one of: primary, secondary, success, danger")
	}
	return nil
}

func validateAppFeedButtons(card map[string]interface{}) error {
	buttonsObj, ok := card["buttons"].(map[string]interface{})
	if !ok || len(buttonsObj) == 0 {
		return nil
	}
	rawButtons, ok := buttonsObj["buttons"].([]interface{})
	if !ok {
		return output.ErrValidation("buttons.buttons must be a JSON array")
	}
	if len(rawButtons) > 2 {
		return output.ErrValidation("buttons.buttons supports at most 2 buttons (got %d)", len(rawButtons))
	}
	for i, raw := range rawButtons {
		button, ok := raw.(map[string]interface{})
		if !ok {
			return output.ErrValidation("buttons.buttons[%d] must be a JSON object", i)
		}
		if err := validateAppFeedButton(i, button); err != nil {
			return err
		}
	}
	return nil
}

func validateAppFeedButton(index int, button map[string]interface{}) error {
	actionType, _ := button["action_type"].(string)
	if actionType == "" {
		return output.ErrValidation("buttons.buttons[%d].action_type is required", index)
	}
	if !oneOf(actionType, "url_page", "webhook") {
		return output.ErrValidation("buttons.buttons[%d].action_type must be url_page or webhook", index)
	}
	textObj, _ := button["text"].(map[string]interface{})
	text, _ := textObj["text"].(string)
	if strings.TrimSpace(text) == "" {
		return output.ErrValidation("buttons.buttons[%d].text.text is required", index)
	}
	if buttonType, _ := button["button_type"].(string); buttonType != "" && !oneOf(buttonType, "default", "primary", "success") {
		return output.ErrValidation("buttons.buttons[%d].button_type must be one of: default, primary, success", index)
	}
	if multiURL, _ := button["multi_url"].(map[string]interface{}); len(multiURL) > 0 {
		hasURL := false
		for _, key := range []string{"url", "android_url", "ios_url", "pc_url"} {
			rawValue, exists := multiURL[key]
			if !exists {
				continue
			}
			raw, ok := rawValue.(string)
			if !ok {
				return output.ErrValidation("buttons.buttons[%d].multi_url.%s must be a string", index, key)
			}
			if strings.TrimSpace(raw) != "" {
				hasURL = true
				if err := validateAppFeedURL(fmt.Sprintf("buttons.buttons[%d].multi_url.%s", index, key), raw, false); err != nil {
					return err
				}
			}
		}
		if actionType == "url_page" && !hasURL {
			return output.ErrValidation("buttons.buttons[%d].multi_url must contain at least one HTTPS URL when action_type is url_page", index)
		}
	}
	if actionType == "url_page" {
		multiURL, _ := button["multi_url"].(map[string]interface{})
		if len(multiURL) == 0 {
			return output.ErrValidation("buttons.buttons[%d].multi_url is required when action_type is url_page", index)
		}
	}
	return nil
}

func validateAppFeedURL(fieldName, raw string, allowAppLink bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" {
		return output.ErrValidation("%s must be a valid URL", fieldName)
	}
	switch u.Scheme {
	case "https":
		if u.Host == "" {
			return output.ErrValidation("%s must be a valid HTTPS URL", fieldName)
		}
		return nil
	case "applink":
		if allowAppLink {
			return nil
		}
	}
	if allowAppLink {
		return output.ErrValidation("%s must use HTTPS or applink", fieldName)
	}
	return output.ErrValidation("%s must use HTTPS", fieldName)
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func cloneAppFeedValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(v))
		for key, item := range v {
			cloned[key] = cloneAppFeedValue(item)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(v))
		for i, item := range v {
			cloned[i] = cloneAppFeedValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), v...)
	default:
		return v
	}
}

func deriveAppFeedCardUpdateFields(card map[string]interface{}) []string {
	fields := make([]string, 0, len(card))
	for _, field := range []string{"title", "avatar_key", "preview", "status_label", "buttons", "link", "time_sensitive", "notify"} {
		if _, ok := card[field]; ok {
			fields = append(fields, appFeedCardUpdateFieldCodes[field])
		}
	}
	return fields
}

var appFeedCardUpdateFieldCodes = map[string]string{
	"1":                       "1",
	"title":                   "1",
	"2":                       "2",
	"avatar":                  "2",
	"avatar-key":              "2",
	"avatar_key":              "2",
	"3":                       "3",
	"preview":                 "3",
	"10":                      "10",
	"status-label":            "10",
	"status_label":            "10",
	"11":                      "11",
	"button":                  "11",
	"buttons":                 "11",
	"12":                      "12",
	"link":                    "12",
	"13":                      "13",
	"time-sensitive":          "13",
	"time_sensitive":          "13",
	"101":                     "101",
	"display-time-to-current": "101",
	"display_time_to_current": "101",
	"102":                     "102",
	"rerank-to-current":       "102",
	"rerank_time_to_current":  "102",
	"rerank_to_current":       "102",
	"103":                     "103",
	"notify":                  "103",
	"with-notify":             "103",
	"with_notify":             "103",
}

func normalizeAppFeedCardUpdateFields(fields []string) ([]string, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field))
		code, ok := appFeedCardUpdateFieldCodes[key]
		if !ok {
			return nil, output.ErrValidation("invalid --update-fields value %q; valid values include title, avatar_key, preview, status_label, buttons, link, time_sensitive, notify, 1, 2, 3, 10, 11, 12, 13, 101, 102, 103", field)
		}
		result = append(result, code)
	}
	return result, nil
}

func stringField(m map[string]interface{}, key string) string {
	value, _ := m[key].(string)
	return value
}

func stringSliceField(m map[string]interface{}, key string) []string {
	raw, ok := m[key].([]interface{})
	if !ok {
		if values, ok := m[key].([]string); ok {
			return values
		}
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok {
			return nil
		}
		values = append(values, value)
	}
	return values
}

func flagChanged(runtime *common.RuntimeContext, name string) bool {
	flag := runtime.Cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

func normalizeAppFeedFailedCards(raw interface{}) []map[string]interface{} {
	items, _ := raw.([]interface{})
	if len(items) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

func outputAppFeedCardBatchResult(runtime *common.RuntimeContext, body map[string]interface{}, resData map[string]interface{}) error {
	feedCards, _ := body["feed_cards"].([]interface{})
	failedCards := normalizeAppFeedFailedCards(resData["failed_cards"])
	out := map[string]interface{}{
		"requested_card_count": len(feedCards),
		"failed_count":         len(failedCards),
		"failed_cards":         failedCards,
	}
	runtime.OutFormat(out, nil, func(w io.Writer) {
		output.PrintTable(w, []map[string]interface{}{{
			"requested_card_count": out["requested_card_count"],
			"failed_count":         out["failed_count"],
		}})
		if len(failedCards) > 0 {
			fmt.Fprintln(w, "\nFailed cards:")
			output.PrintTable(w, failedCards)
		}
	})
	return nil
}
