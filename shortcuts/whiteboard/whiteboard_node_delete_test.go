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

func TestWhiteboardNodeDeleteValidate_InvalidNodeIDsTypedParam(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-board",
		"node-ids":         "nodeA,,nodeB",
	}, nil)

	err := wbNodeDeleteValidate(context.Background(), rt)
	assertValidationParam(t, err, "--node-ids", false)
}

func TestWhiteboardNodeDeleteMetadata_RiskHighRiskWrite(t *testing.T) {
	t.Parallel()

	if WhiteboardNodeDelete.Risk != "high-risk-write" {
		t.Fatalf("Risk = %q, want high-risk-write", WhiteboardNodeDelete.Risk)
	}
}

func TestWhiteboardNodeDeleteDryRun_RequestShape(t *testing.T) {
	t.Parallel()

	rt := newTestRuntime(map[string]string{
		"whiteboard-token": "test-board",
		"node-ids":         "nodeA,nodeB",
		"idempotent-token": "delete-token-12345",
	}, nil)

	dryRun := wbNodeDeleteDryRun(context.Background(), rt)
	if dryRun == nil {
		t.Fatal("wbNodeDeleteDryRun() returned nil")
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
	if got.API[0].Method != "DELETE" {
		t.Fatalf("method = %q, want DELETE", got.API[0].Method)
	}
	if got.API[0].URL != "/open-apis/board/v1/whiteboards/test...oard/nodes/batch_delete" {
		t.Fatalf("url = %q, want masked node-delete URL", got.API[0].URL)
	}
	if got.API[0].Params["client_token"] != "delete-token-12345" {
		t.Fatalf("params.client_token = %#v, want delete-token-12345", got.API[0].Params["client_token"])
	}
	ids, ok := got.API[0].Body["ids"].([]interface{})
	if !ok || len(ids) != 2 {
		t.Fatalf("body.ids = %#v, want two ids", got.API[0].Body["ids"])
	}
	if ids[0] != "nodeA" || ids[1] != "nodeB" {
		t.Fatalf("body.ids = %#v, want [nodeA nodeB]", ids)
	}
}

func TestWhiteboardNodeDeleteExecute_PostsIDs(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	stub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_delete",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	args := []string{"+node-delete", "--whiteboard-token", "test-board", "--node-ids", "nodeA,nodeB"}
	if err := runUpdateShortcut(t, WhiteboardNodeDelete, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("unmarshal captured body: %v\nraw=%s", err, string(stub.CapturedBody))
	}
	ids, ok := body["ids"].([]interface{})
	if !ok || len(ids) != 2 {
		t.Fatalf("body.ids = %#v, want two ids; body=%s", body["ids"], string(stub.CapturedBody))
	}
	if ids[0] != "nodeA" || ids[1] != "nodeB" {
		t.Fatalf("body.ids = %#v, want [nodeA nodeB]", ids)
	}
	if !strings.Contains(stdout.String(), `"ids": "nodeA,nodeB"`) {
		t.Fatalf("stdout=%s, want ids nodeA,nodeB", stdout.String())
	}
}

func TestWhiteboardNodeDeleteExecute_RejectsNonObjectSuccessResponse(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	reg.Register(&httpmock.Stub{
		Method:  "DELETE",
		URL:     "/open-apis/board/v1/whiteboards/test-board/nodes/batch_delete",
		RawBody: []byte("[]"),
	})

	args := []string{"+node-delete", "--whiteboard-token", "test-board", "--node-ids", "nodeA"}
	err := runUpdateShortcut(t, WhiteboardNodeDelete, args, factory, stdout)
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

func TestWhiteboardNodeDeleteExecute_PreservesIdempotentToken(t *testing.T) {
	factory, stdout, reg := newUpdateExecuteFactory(t)

	const token = "         x"
	var capturedToken string
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/board/v1/whiteboards/test-board/nodes/batch_delete",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
		OnMatch: func(req *http.Request) {
			capturedToken = req.URL.Query().Get("client_token")
		},
	})

	args := []string{"+node-delete", "--whiteboard-token", "test-board", "--node-ids", "nodeA", "--idempotent-token", token}
	if err := runUpdateShortcut(t, WhiteboardNodeDelete, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if capturedToken != token {
		t.Fatalf("client_token = %q, want %q", capturedToken, token)
	}
}
