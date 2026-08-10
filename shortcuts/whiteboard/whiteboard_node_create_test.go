// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestWhiteboardNodeCreateValidate_InvalidSourceTypedParam(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-board",
		"source":           "not-json",
	}, nil)

	err := wbNodeCreateValidate(context.Background(), rt)
	assertValidationParam(t, err, "--source", true)
}

func TestWhiteboardNodeCreateDryRun_RequestShape(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-board",
		"idempotent-token": "create-token-12345",
		"source":           `{"nodes":[{"id":"tmpNode","type":"composite_shape","x":0,"y":0,"width":260,"height":45,"text":{"text":"hello","font_weight":"regular","font_size":14,"horizontal_align":"center","vertical_align":"mid"},"style":{"border_color":"#3370ff","border_width":"narrow","border_style":"solid","fill_color":"#e8f3ff"},"composite_shape":{"type":"round_rect"}}]}`,
	}, nil)

	dryRun := wbNodeCreateDryRun(context.Background(), rt)
	if dryRun == nil {
		t.Fatal("wbNodeCreateDryRun() returned nil")
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
	if got.API[0].Method != "POST" {
		t.Fatalf("method = %q, want POST", got.API[0].Method)
	}
	if got.API[0].URL != "/open-apis/board/v1/whiteboards/test...oard/nodes" {
		t.Fatalf("url = %q, want node-create URL", got.API[0].URL)
	}
	if got.API[0].Params["client_token"] != "create-token-12345" {
		t.Fatalf("params.client_token = %#v, want create-token-12345", got.API[0].Params["client_token"])
	}
	nodes, ok := got.API[0].Body["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Fatalf("body.nodes = %#v, want one node", got.API[0].Body["nodes"])
	}
	node, ok := nodes[0].(map[string]interface{})
	if !ok || node["type"] != "composite_shape" {
		t.Fatalf("body.nodes[0] = %#v, want type composite_shape", nodes[0])
	}
	if _, ok := node["composite_shape"].(map[string]interface{}); !ok {
		t.Fatalf("body.nodes[0].composite_shape = %#v, want object", node["composite_shape"])
	}
}

func TestWhiteboardNodeCreateExecute_PostsNodes(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []string{"node-1"},
			},
		},
	}
	reg.Register(stub)

	source := `{"nodes":[{"id":"tmpNode","type":"composite_shape","x":0,"y":0,"width":260,"height":45,"text":{"text":"hello","font_weight":"regular","font_size":14,"horizontal_align":"center","vertical_align":"mid"},"style":{"border_color":"#3370ff","border_width":"narrow","border_style":"solid","fill_color":"#e8f3ff"},"composite_shape":{"type":"round_rect"}}]}`
	args := []string{"+node-create", "--whiteboard-token", "test-board", "--source", source}
	if err := runUpdateShortcut(t, WhiteboardNodeCreate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	nodes, ok := body["nodes"].([]interface{})
	if !ok || len(nodes) != 1 {
		t.Fatalf("body.nodes = %#v, want one node; body=%s", body["nodes"], string(stub.CapturedBody))
	}
	node, ok := nodes[0].(map[string]interface{})
	if !ok || node["type"] != "composite_shape" {
		t.Fatalf("body.nodes[0] = %#v, want type composite_shape", nodes[0])
	}
	if _, ok := node["composite_shape"].(map[string]interface{}); !ok {
		t.Fatalf("body.nodes[0].composite_shape = %#v, want object", node["composite_shape"])
	}
	if !strings.Contains(stdout.String(), `"ids": "node-1"`) {
		t.Fatalf("stdout=%s, want ids node-1", stdout.String())
	}
}

func TestWhiteboardNodeCreateExecute_RejectsMissingIDs(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	})

	source := `{"nodes":[{"id":"tmpNode","type":"composite_shape","x":0,"y":0,"width":260,"height":45,"text":{"text":"hello","font_weight":"regular","font_size":14,"horizontal_align":"center","vertical_align":"mid"},"style":{"border_color":"#3370ff","border_width":"narrow","border_style":"solid","fill_color":"#e8f3ff"},"composite_shape":{"type":"round_rect"}}]}`
	args := []string{"+node-create", "--whiteboard-token", "test-board", "--source", source}
	err := runUpdateShortcut(t, WhiteboardNodeCreate, args, factory, stdout)
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

func TestWhiteboardNodeCreateIDs_RejectsEmptyIDs(t *testing.T) {
	t.Parallel()

	_, err := whiteboardNodeCreateIDs(map[string]interface{}{"ids": []interface{}{}})
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
	}
}

func TestWhiteboardNodeCreateExecute_RejectsMalformedIDs(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ids": []interface{}{"node-1", 2},
			},
		},
	})

	source := `{"nodes":[{"id":"tmpNode","type":"composite_shape","x":0,"y":0,"width":260,"height":45,"text":{"text":"hello","font_weight":"regular","font_size":14,"horizontal_align":"center","vertical_align":"mid"},"style":{"border_color":"#3370ff","border_width":"narrow","border_style":"solid","fill_color":"#e8f3ff"},"composite_shape":{"type":"round_rect"}}]}`
	args := []string{"+node-create", "--whiteboard-token", "test-board", "--source", source}
	err := runUpdateShortcut(t, WhiteboardNodeCreate, args, factory, stdout)
	if err == nil {
		t.Fatal("expected malformed ids error, got nil")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
	if internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("Subtype = %q, want %q", internalErr.Subtype, errs.SubtypeInvalidResponse)
	}
}
