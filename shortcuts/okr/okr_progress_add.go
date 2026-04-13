// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

// buildProgressBody constructs the request body for creating a progress record.
// Precedence: --data provides the base, individual flags override it.
func buildProgressBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Step 1: --data provides the base payload
	if dataStr := runtime.Str("data"); dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &body); err != nil {
			return nil, fmt.Errorf("--data must be a valid JSON object: %v", err)
		}
	}

	// Step 2: individual flags override --data values
	if targetID := runtime.Str("target-id"); targetID != "" {
		body["target_id"] = targetID
	}
	if targetType := runtime.Str("target-type"); targetType != "" {
		resolved, err := resolveTargetType(targetType)
		if err != nil {
			return nil, err
		}
		body["target_type"] = resolved
	}

	if text := runtime.Str("text"); text != "" {
		body["content"] = wrapPlainTextContent(text)
	} else if contentStr := runtime.Str("content"); contentStr != "" {
		var content interface{}
		if err := json.Unmarshal([]byte(contentStr), &content); err != nil {
			return nil, fmt.Errorf("--content must be a valid JSON object: %v", err)
		}
		body["content"] = content
	}

	if sourceTitle := runtime.Str("source-title"); sourceTitle != "" {
		body["source_title"] = sourceTitle
	} else if _, ok := body["source_title"]; !ok {
		body["source_title"] = "lark-cli"
	}

	if sourceURL := runtime.Str("source-url"); sourceURL != "" {
		body["source_url"] = sourceURL
	} else if _, ok := body["source_url"]; !ok {
		body["source_url"] = "https://github.com/larksuite/cli"
	}

	return body, nil
}

// AddProgress adds a progress record to an objective or key result.
var AddProgress = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-add",
	Description: "add a progress record to an objective or key result",
	Risk:        "write",
	Scopes:      []string{"okr:okr.progress:writeonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "target-id", Desc: "target objective or key result ID (required unless provided via --data)"},
		{Name: "target-type", Desc: "target type: objective or key_result (required unless provided via --data)", Enum: []string{"objective", "key_result", "2", "3"}},
		{Name: "text", Desc: "plain text progress content (auto-converted to rich text)"},
		{Name: "content", Desc: "rich text JSON content", Input: []string{common.File, common.Stdin}},
		{Name: "source-title", Desc: "source title (default: lark-cli)"},
		{Name: "source-url", Desc: "source URL (default: https://github.com/larksuite/cli)"},
		{Name: "data", Desc: "full JSON payload; individual flags override fields in --data"},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data := runtime.Str("data")
		text := runtime.Str("text")
		content := runtime.Str("content")

		// Content is required via --text, --content, or --data
		if text == "" && content == "" && data == "" {
			return fmt.Errorf("one of --text, --content, or --data is required")
		}

		// target-id and target-type are required unless --data provides them
		if data == "" {
			if runtime.Str("target-id") == "" {
				return fmt.Errorf("--target-id is required (or provide via --data)")
			}
			if runtime.Str("target-type") == "" {
				return fmt.Errorf("--target-type is required (or provide via --data)")
			}
		}
		return nil
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildProgressBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/okr/v1/progress_records").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildProgressBody(runtime)
		if err != nil {
			return WrapOkrError(ErrCodeOkrInvalidParams, err.Error(), "build progress body")
		}

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodPost,
			ApiPath:     "/open-apis/okr/v1/progress_records",
			QueryParams: queryParams,
			Body:        body,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse progress response")
			}
		}

		data, err := HandleOkrApiResult(result, err, "add progress record")
		if err != nil {
			return err
		}

		progressID, _ := data["progress_id"].(string)

		outData := map[string]interface{}{
			"progress_id": progressID,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Progress record added successfully!")
			if progressID != "" {
				fmt.Fprintf(w, "Progress ID: %s\n", progressID)
			}
		})
		return nil
	},
}
