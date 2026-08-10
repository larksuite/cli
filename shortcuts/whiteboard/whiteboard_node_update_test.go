// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestWhiteboardNodeUpdateValidate_SourceMissingIDTypedParam(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-board",
		"source":           `{"nodes":[{"type":"text_shape","text":{"text":"hello"}}]}`,
	}, nil)

	err := wbNodeUpdateValidate(context.Background(), rt)
	assertValidationParam(t, err, "--source", false)
}

func TestWhiteboardNodeUpdateDryRun_RequestShape(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-board",
		"idempotent-token": "update-token-12345",
		"source": `{"nodes":[` +
			`{"id":"nodeA","type":"future_gateway_type","text":{"text":"hello A"}},` +
			`{"id":"nodeB","type":"text_shape","text":{"text":"hello B"}}` +
			`]}`,
	}, nil)

	dryRun := wbNodeUpdateDryRun(context.Background(), rt)
	if dryRun == nil {
		t.Fatal("wbNodeUpdateDryRun() returned nil")
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	data, err := json.Marshal(dryRun)
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry-run: %v\njson=%s", err, string(data))
	}
	if len(got.API) != 1 {
		t.Fatalf("api len = %d, want 1; json=%s", len(got.API), string(data))
	}
	if got.API[0].Method != "PUT" {
		t.Fatalf("method = %q, want PUT", got.API[0].Method)
	}
	if got.API[0].URL != "/open-apis/board/v1/whiteboards/test...oard/nodes/batch_update" {
		t.Fatalf("url = %q, want masked batch_update URL", got.API[0].URL)
	}
	if got.API[0].Params["client_token"] != "update-token-12345" {
		t.Fatalf("params.client_token = %#v, want update-token-12345", got.API[0].Params["client_token"])
	}
	nodes, ok := got.API[0].Body["nodes"].([]interface{})
	if !ok || len(nodes) != 2 {
		t.Fatalf("body.nodes = %#v, want two nodes", got.API[0].Body["nodes"])
	}
	wantText := []string{"hello A", "hello B"}
	for i := range nodes {
		node, ok := nodes[i].(map[string]interface{})
		if !ok {
			t.Fatalf("body.nodes[%d] = %T, want map; nodes=%#v", i, nodes[i], nodes)
		}
		if node["id"] != []string{"nodeA", "nodeB"}[i] {
			t.Fatalf("body.nodes[%d].id = %#v", i, node["id"])
		}
		wantType := []string{"future_gateway_type", "text_shape"}[i]
		if node["type"] != wantType {
			t.Fatalf("body.nodes[%d].type = %#v, want %q", i, node["type"], wantType)
		}
		text, ok := node["text"].(map[string]interface{})
		if !ok || text["text"] != wantText[i] {
			t.Fatalf("body.nodes[%d].text = %#v, want text %q", i, node["text"], wantText[i])
		}
	}
}

func TestWhiteboardNodeUpdateExecute_RejectsMissingOrInvalidTypeBeforeHTTP(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "missing", source: `{"nodes":[{"id":"nodeA"}]}`},
		{name: "empty", source: `{"nodes":[{"id":"nodeA","type":""}]}`},
		{name: "blank", source: `{"nodes":[{"id":"nodeA","type":"   "}]}`},
		{name: "non-string", source: `{"nodes":[{"id":"nodeA","type":42}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newUpdateExecuteFactory(t)
			requestCount := 0
			reg.Register(&httpmock.Stub{
				Method:   http.MethodPut,
				URL:      "/open-apis/board/v1/whiteboards/test-board/nodes/batch_update",
				Optional: true,
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "success",
					"data": map[string]interface{}{"ids": []string{"nodeA"}},
				},
				OnMatch: func(*http.Request) { requestCount++ },
			})

			err := runUpdateShortcut(t, WhiteboardNodeUpdate, []string{
				"+node-update",
				"--whiteboard-token", "test-board",
				"--source", tt.source,
			}, factory, stdout)

			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error type = %T, want *errs.ValidationError", err)
			}
			if validationErr.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("Subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
			}
			if validationErr.Param != "--source" {
				t.Fatalf("Param = %q, want --source", validationErr.Param)
			}
			if len(validationErr.Params) != 1 {
				t.Fatalf("Params = %#v, want one item", validationErr.Params)
			}
			if got, want := validationErr.Params[0].Name, "nodes[0].type"; got != want {
				t.Fatalf("Params[0].Name = %q, want %q", got, want)
			}
			if got, want := validationErr.Params[0].Reason, "required by the current whiteboard.node gateway schema"; got != want {
				t.Fatalf("Params[0].Reason = %q, want %q", got, want)
			}
			if requestCount != 0 {
				t.Fatalf("HTTP request count = %d, want 0", requestCount)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestWhiteboardNodeUpdateBody_PreservesV1DataTypeFields(t *testing.T) {
	t.Parallel()

	payload, err := parseWhiteboardNodeBatchPayload([]byte(`{"nodes":[{
		"id":"nodeA",
		"type":"text_shape",
		"parent_id":"parentA",
		"x":1,
		"y":2,
		"width":100,
		"height":50,
		"locked":false,
		"z_index":3,
		"children":["childA"],
		"created_at":123,
		"unknown_top":{"x":1},
		"text":{
			"text":"hello",
			"content":"legacy alias must not be sent",
			"font_size":14,
			"theme_text_color_code":2,
			"theme_text_background_color_code":3,
			"text_color_type":1,
			"text_background_color_type":0,
			"dark_text_color":"#111111",
			"dark_text_background_color":"#222222",
			"dark_theme_text_color_code":4,
			"dark_theme_text_background_color_code":5,
			"rich_text":{
				"paragraphs":[{
					"paragraph_type":0,
					"elements":[{
						"element_type":0,
						"text_element":{
							"text":"hello",
							"text_style":{
								"font_size":14,
								"dark_text_color":"#333333",
								"dark_text_background_color":"#444444",
								"extra_style":true
							}
						},
						"extra_element":true
					}],
					"extra_paragraph":true
				}],
				"extra_rich_text":true
			}
		},
		"style":{
			"fill_color":"#ffffff",
			"theme_fill_color_code":6,
			"theme_border_color_code":7,
			"fill_color_type":1,
			"border_color_type":0,
			"dark_fill_color":"#000000",
			"dark_border_color":"#010101",
			"dark_theme_fill_color_code":8,
			"dark_theme_border_color_code":9,
			"border_dasharrays":[4,2],
			"border_radius":{"top_left":4,"unexpected":9},
			"shadow":{"color":"#999999","blur":8,"offset_x":1,"offset_y":2,"opacity":0.5,"extra":1},
			"fill_gradient":{
				"type":"linear-gradient",
				"handle_positions":[{"x":0,"y":0,"extra":1}],
				"stops":[{"position":0,"color":"#fff","extra":1}]
			},
			"extra_style":true
		},
		"connector":{
			"turning_points":[{"x":1,"y":2,"extra":3}],
			"start_object":{"id":"nodeB","position":{"x":4,"y":5,"extra":6},"extra_object":true},
			"extra_connector":true
		},
		"syntax":{"syntax_type":"svg","code":"<svg/>","style_type":"default","extra_syntax":true}
	}]}`), true)
	if err != nil {
		t.Fatalf("parseWhiteboardNodeBatchPayload() error = %v", err)
	}

	body := whiteboardNodeBatchUpdateBody(payload)
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(data)
	for _, banned := range []string{
		"created_at",
		"unknown_top",
		"content",
		"extra_style",
		"extra_rich_text",
		"extra_paragraph",
		"extra_element",
		"unexpected",
		"extra_connector",
		"extra_object",
		"extra_syntax",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("sanitized body still contains %q: %s", banned, got)
		}
	}
	for _, want := range []string{
		`"id":"nodeA"`,
		`"parent_id":"parentA"`,
		`"text":"hello"`,
		`"font_size":14`,
		`"theme_text_color_code":2`,
		`"theme_text_background_color_code":3`,
		`"text_color_type":1`,
		`"text_background_color_type":0`,
		`"dark_text_color":"#111111"`,
		`"dark_text_background_color":"#222222"`,
		`"dark_theme_text_color_code":4`,
		`"dark_theme_text_background_color_code":5`,
		`"dark_text_color":"#333333"`,
		`"dark_text_background_color":"#444444"`,
		`"theme_fill_color_code":6`,
		`"theme_border_color_code":7`,
		`"fill_color_type":1`,
		`"border_color_type":0`,
		`"dark_fill_color":"#000000"`,
		`"dark_border_color":"#010101"`,
		`"dark_theme_fill_color_code":8`,
		`"dark_theme_border_color_code":9`,
		`"border_dasharrays":[4,2]`,
		`"border_radius":{"top_left":4}`,
		`"shadow":{"blur":8,"color":"#999999","offset_x":1,"offset_y":2,"opacity":0.5}`,
		`"fill_gradient"`,
		`"turning_points":[{"x":1,"y":2}]`,
		`"syntax":{"code":"\u003csvg/\u003e","style_type":"default","syntax_type":"svg"}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sanitized body missing %q: %s", want, got)
		}
	}
}

func TestWhiteboardNodeUpdateExecute_BatchUpdatesNodes(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []string{"nodeA", "nodeB"},
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
	}
	reg.Register(stub)

	source := `{"nodes":[` +
		`{"id":"nodeA","type":"text_shape","text":{"text":"hello A"}},` +
		`{"id":"nodeB","type":"text_shape","text":{"text":"hello B"}}` +
		`]}`
	args := []string{"+node-update", "--whiteboard-token", "test-board", "--source", source, "--idempotent-token", "update-token-12345"}
	if err := runUpdateShortcut(t, WhiteboardNodeUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	assertNodeBatchUpdateCapturedBody(t, stub.CapturedBody, []string{"hello A", "hello B"})
	if !strings.Contains(capturedQuery, "client_token=update-token-12345") {
		t.Fatalf("query = %q, want client_token", capturedQuery)
	}
	if !strings.Contains(stdout.String(), `"ids": "nodeA,nodeB"`) {
		t.Fatalf("stdout=%s, want ids nodeA,nodeB", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"count": 2`) {
		t.Fatalf("stdout=%s, want count 2", stdout.String())
	}
}

func TestWhiteboardNodeUpdateExecute_WithoutIdempotentTokenOmitsClientToken(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	var capturedQuery string
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []string{"nodeA"},
			},
		},
		OnMatch: func(req *http.Request) {
			capturedQuery = req.URL.RawQuery
		},
	}
	reg.Register(stub)

	source := `{"nodes":[{"id":"nodeA","type":"text_shape","text":{"text":"hello A"}}]}`
	args := []string{"+node-update", "--whiteboard-token", "test-board", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardNodeUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if capturedQuery != "" {
		t.Fatalf("query = %q, want empty when --idempotent-token is absent", capturedQuery)
	}
}

func TestWhiteboardNodeUpdateExecute_BatchFailureReturnsAPIError(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_update",
		Body: map[string]interface{}{
			"code": 2890002,
			"msg":  "text color code cannot be empty while type is system [@from@] arg error [@from@] whiteboard",
			"error": map[string]interface{}{
				"message":        "Invalid request parameter: text_color_code. Invalid reason : text_color_code is required when text_color_type is system. Please check and modify accordingly.",
				"log_id":         "whiteboard-log-2890002",
				"troubleshooter": "https://open.feishu.cn/document/troubleshooter/whiteboard-node",
			},
		},
	})

	source := `{"nodes":[` +
		`{"id":"nodeA","type":"text_shape","text":{"text":"hello A"}},` +
		`{"id":"nodeB","type":"text_shape","text":{"text":"hello B"}}` +
		`]}`
	args := []string{"+node-update", "--whiteboard-token", "test-board", "--source", source}
	err := runUpdateShortcut(t, WhiteboardNodeUpdate, args, factory, stdout)
	if err == nil {
		t.Fatal("expected batch update failure error, got nil")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T: %v, want *errs.APIError reachable via errors.As", err, err)
	}
	if apiErr.Category != errs.CategoryAPI || apiErr.Subtype != errs.SubtypeInvalidParameters {
		t.Fatalf("problem = %+v, want api/invalid_parameters", apiErr.Problem)
	}
	if apiErr.Code != 2890002 {
		t.Fatalf("Code = %d, want 2890002", apiErr.Code)
	}
	if apiErr.Message != "text color code cannot be empty while type is system [@from@] arg error [@from@] whiteboard" {
		t.Fatalf("Message = %q", apiErr.Message)
	}
	if apiErr.LogID != "whiteboard-log-2890002" {
		t.Fatalf("LogID = %q", apiErr.LogID)
	}
	if apiErr.Troubleshooter != "https://open.feishu.cn/document/troubleshooter/whiteboard-node" {
		t.Fatalf("Troubleshooter = %q", apiErr.Troubleshooter)
	}
	if got, want := apiErr.Hint, "Invalid request parameter: text_color_code. Invalid reason : text_color_code is required when text_color_type is system. Please check and modify accordingly."; got != want {
		t.Fatalf("Hint = %q, want %q", got, want)
	}
	if !apiErr.HintIsFromServer() {
		t.Fatal("error.message hint must retain server provenance")
	}
	if len(apiErr.FieldViolations) != 0 {
		t.Fatalf("FieldViolations = %#v, want none in standard OGW envelope", apiErr.FieldViolations)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no success output", stdout.String())
	}
}

func TestWhiteboardNodeUpdateExecute_FieldViolationsReturnAPIError(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_update",
		Body: map[string]interface{}{
			"code":   99992402,
			"msg":    "field validation failed",
			"log_id": "whiteboard-log-99992402",
			"error": map[string]interface{}{
				"field_violations": []interface{}{
					map[string]interface{}{
						"field":       "nodes[0].type",
						"value":       "42",
						"description": "type must match the node payload",
					},
				},
			},
		},
	})

	source := "{\"nodes\":[{\"id\":\"nodeA\",\"type\":\"text_shape\",\"text\":{\"text\":\"hello A\"}}]}"
	err := runUpdateShortcut(t, WhiteboardNodeUpdate, []string{
		"+node-update", "--whiteboard-token", "test-board", "--source", source,
	}, factory, stdout)
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T: %v, want *errs.APIError", err, err)
	}
	if apiErr.Code != 99992402 || apiErr.Subtype != errs.SubtypeInvalidParameters {
		t.Fatalf("problem = %+v, want code 99992402 api/invalid_parameters", apiErr.Problem)
	}
	if got, want := apiErr.Hint, "nodes[0].type: type must match the node payload"; got != want {
		t.Fatalf("Hint = %q, want %q", got, want)
	}
	if !apiErr.HintIsFromServer() {
		t.Fatal("field violation hint must retain server provenance")
	}
	if len(apiErr.FieldViolations) != 1 {
		t.Fatalf("FieldViolations = %#v, want one item", apiErr.FieldViolations)
	}
	violation := apiErr.FieldViolations[0]
	if violation.Field != "nodes[0].type" || violation.Value != "42" || violation.Description != "type must match the node payload" {
		t.Fatalf("FieldViolations[0] = %#v", violation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no success output", stdout.String())
	}
}

func TestWhiteboardNodeUpdateExecute_RejectsMissingIDs(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	source := `{"nodes":[{"id":"nodeA","type":"text_shape","text":{"text":"hello A"}}]}`
	args := []string{"+node-update", "--whiteboard-token", "test-board", "--source", source}
	err := runUpdateShortcut(t, WhiteboardNodeUpdate, args, factory, stdout)
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
	}
	if strings.Contains(stdout.String(), "success") {
		t.Fatalf("stdout=%s, must not report success", stdout.String())
	}
}

func TestWhiteboardNodeUpdateIDs_RejectsEmptyIDs(t *testing.T) {
	t.Parallel()

	_, err := whiteboardNodeUpdateIDs(map[string]interface{}{"ids": []interface{}{}})
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
	}
}

func TestWhiteboardNodeUpdateTips_MentionTemporaryNonAtomicBehavior(t *testing.T) {
	t.Parallel()

	tips := strings.Join(WhiteboardNodeUpdate.Tips, "\n")
	for _, want := range []string{"batch_update", "client_token", "one whiteboard.node batch_update request"} {
		if !strings.Contains(tips, want) {
			t.Fatalf("tips = %q, want substring %q", tips, want)
		}
	}
	for _, banned := range []string{"fans out", "non-atomic", "Temporary behavior"} {
		if strings.Contains(tips, banned) {
			t.Fatalf("tips = %q, should not contain old fan-out wording %q", tips, banned)
		}
	}
}

func assertNodeBatchUpdateCapturedBody(t *testing.T, raw []byte, wantContent []string) {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v\nraw=%s", err, string(raw))
	}
	nodes, ok := body["nodes"].([]interface{})
	if !ok || len(nodes) != len(wantContent) {
		t.Fatalf("body.nodes = %#v, want %d nodes; body=%s", body["nodes"], len(wantContent), string(raw))
	}
	for i, rawNode := range nodes {
		node, ok := rawNode.(map[string]interface{})
		if !ok {
			t.Fatalf("body.nodes[%d] = %T, want map; body=%s", i, rawNode, string(raw))
		}
		if _, exists := node["id"]; !exists {
			t.Fatalf("body.nodes[%d].id absent; body=%s", i, string(raw))
		}
		text, ok := node["text"].(map[string]interface{})
		if !ok || text["text"] != wantContent[i] {
			t.Fatalf("body.nodes[%d].text = %#v, want text %q; body=%s", i, node["text"], wantContent[i], string(raw))
		}
	}
}
