// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const feedCardTimeSensitiveBasePath = "/open-apis/im/v2/feed_cards"
const feedCardTimeSensitivePathTemplate = feedCardTimeSensitiveBasePath + "/:feed_card_id"

const feedCardTimeSensitiveScope = "im:datasync.feed_card.time_sensitive:write"

var ImFeedCardTimeSensitive = common.Shortcut{
	Service:     "im",
	Command:     "+feed-card-time-sensitive",
	Description: "Set a feed card as temporarily pinned or unpinned for users by feed_card_id; bot only",
	Risk:        "write",
	Scopes:      []string{feedCardTimeSensitiveScope},
	AuthTypes:   []string{"bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "feed-card-id", Desc: "feed_card_id to update", Required: true},
		{Name: "user-ids", Desc: "recipient user IDs, comma-separated; IDs must match --user-id-type", Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "recipient ID type", Enum: []string{"open_id", "union_id", "user_id"}},
		{Name: "time-sensitive", Desc: "temporary pin status", Required: true, Enum: []string{"true", "false"}},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildFeedCardTimeSensitiveBody(runtime)
		return common.NewDryRunAPI().
			PATCH(feedCardTimeSensitivePathTemplate).
			Set("feed_card_id", strings.TrimSpace(runtime.Str("feed-card-id"))).
			Params(map[string]interface{}{"user_id_type": appFeedCardUserIDType(runtime)}).
			Body(body)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := feedCardIDForTimeSensitive(runtime); err != nil {
			return err
		}
		_, err := buildFeedCardTimeSensitiveBody(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		requestPath, feedCardID, err := feedCardTimeSensitiveRequestPath(runtime)
		if err != nil {
			return err
		}
		body, err := buildFeedCardTimeSensitiveBody(runtime)
		if err != nil {
			return err
		}
		if err := doFeedCardTimeSensitivePatch(runtime, requestPath, body); err != nil {
			return err
		}
		return outputFeedCardTimeSensitiveResult(runtime, feedCardID, body)
	},
}

func buildFeedCardTimeSensitiveBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	userIDs := common.SplitCSV(runtime.Str("user-ids"))
	if err := validateAppFeedUserIDs(userIDs, appFeedCardUserIDType(runtime), "--user-ids"); err != nil {
		return nil, err
	}
	timeSensitive, err := feedCardTimeSensitiveValue(runtime)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"user_ids":       userIDs,
		"time_sensitive": timeSensitive,
	}, nil
}

func feedCardTimeSensitiveValue(runtime *common.RuntimeContext) (bool, error) {
	raw := strings.TrimSpace(runtime.Str("time-sensitive"))
	if raw == "" {
		return false, output.ErrValidation("--time-sensitive is required; pass --time-sensitive true to pin or --time-sensitive false to unpin")
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, output.ErrValidation("--time-sensitive must be true or false")
	}
	return value, nil
}

func feedCardIDForTimeSensitive(runtime *common.RuntimeContext) (string, error) {
	feedCardID := strings.TrimSpace(runtime.Str("feed-card-id"))
	if feedCardID == "" {
		return "", output.ErrValidation("--feed-card-id is required")
	}
	if !strings.HasPrefix(feedCardID, "oc_") {
		return "", output.ErrWithHint(output.ExitValidation, "validation",
			`--feed-card-id must be a group feed_card_id starting with "oc_"`,
			`pass a group feed_card_id such as oc_xxx to "lark-cli im +feed-card-time-sensitive"`)
	}
	return feedCardID, nil
}

func feedCardTimeSensitiveRequestPath(runtime *common.RuntimeContext) (string, string, error) {
	feedCardID, err := feedCardIDForTimeSensitive(runtime)
	if err != nil {
		return "", "", err
	}
	return feedCardTimeSensitiveBasePath + "/" + validate.EncodePathSegment(feedCardID), feedCardID, nil
}

func doFeedCardTimeSensitivePatch(runtime *common.RuntimeContext, requestPath string, body map[string]interface{}) error {
	resp, err := runtime.DoAPIStream(runtime.Ctx(), &larkcore.ApiReq{
		HttpMethod: http.MethodPatch,
		ApiPath:    requestPath,
		QueryParams: larkcore.QueryParams{
			"user_id_type": []string{appFeedCardUserIDType(runtime)},
		},
		Body: body,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return output.ErrNetwork("read response body: %v", err)
	}
	if len(strings.TrimSpace(string(rawBody))) == 0 {
		return nil
	}

	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return output.Errorf(output.ExitAPI, "api", "unmarshal response: %v", err)
	}
	if envelope.Code != 0 {
		return output.ErrAPI(envelope.Code, envelope.Msg, nil)
	}
	return nil
}

func outputFeedCardTimeSensitiveResult(runtime *common.RuntimeContext, feedCardID string, body map[string]interface{}) error {
	userIDs, _ := body["user_ids"].([]string)
	out := map[string]interface{}{
		"requested_user_count": len(userIDs),
		"time_sensitive":       body["time_sensitive"],
	}
	if feedCardID != "" {
		out["feed_card_id"] = feedCardID
	}
	runtime.OutFormat(out, nil, func(w io.Writer) {
		output.PrintTable(w, []map[string]interface{}{out})
	})
	return nil
}
