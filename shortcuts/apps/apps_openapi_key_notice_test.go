// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// shownOnceNotice is the one-time-secret guidance shared by create/reset.
const shownOnceNotice = "this api_key is shown only once"

func issuedKeyErrBuffer(t *testing.T, rctx *common.RuntimeContext) *bytes.Buffer {
	t.Helper()
	buf, ok := rctx.IO().ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("ErrOut is not *bytes.Buffer: %T", rctx.IO().ErrOut)
	}
	return buf
}

func registerCreateIssuedKey(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"api_key_id": "k1",
				"info": map[string]interface{}{
					"api_key_id": "k1", "name": "partner-test",
					"api_key": "xxxxxxxxxxxx", "status": float64(1),
				},
			},
		},
	})
}

func registerResetIssuedKey(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1/refresh",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"api_key": "xxxxxxxxxxxx",
				"info":    map[string]interface{}{"api_key_id": "k1", "name": "k", "api_key": "xxxxxxxxxxxx", "status": float64(1)},
			},
		},
	})
}

// In JSON mode the one-time notice rides inside the success envelope and
// nothing is written to stderr, so stdout stays one parseable JSON document
// even when a caller merges the streams with 2>&1.
func TestOpenAPIKeyCreate_JSON_NoticeInEnvelopeNotStderr(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		createFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "partner-test"})
	registerCreateIssuedKey(reg)
	stderrBuf := issuedKeyErrBuffer(t, rctx)

	if err := AppsOpenAPIKeyCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("stdout must be a single pure JSON document: %v\n%s", err, stdoutBuf.String())
	}
	if !env.OK {
		t.Errorf("envelope ok = false, want true")
	}
	notice, _ := env.Data["notice"].(string)
	if !strings.Contains(notice, shownOnceNotice) {
		t.Errorf("data.notice must carry the one-time guidance, got %q", notice)
	}
	if env.Data["api_key"] != "xxxxxxxxxxxx" {
		t.Errorf("data.api_key must carry the raw secret once, got %v", env.Data["api_key"])
	}
	if strings.Contains(stderrBuf.String(), shownOnceNotice) || strings.Contains(stderrBuf.String(), "warning:") {
		t.Errorf("json mode must not print the notice to stderr:\n%s", stderrBuf.String())
	}
	// Even after `2>&1` the merged stream must still parse as JSON — the
	// original bug had the warning prepended and broken decoding.
	merged := append(append([]byte{}, stdoutBuf.Bytes()...), stderrBuf.Bytes()...)
	var mergedEnv map[string]interface{}
	if err := json.Unmarshal(merged, &mergedEnv); err != nil {
		t.Errorf("merged stdout+stderr (2>&1) must still parse as JSON: %v\n%s", err, string(merged))
	}
}

// Reset shares the same output path and must follow the same JSON contract.
func TestOpenAPIKeyReset_JSON_NoticeInEnvelopeNotStderr(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string", "yes": "bool"},
		map[string]string{"app-id": "app_x", "key-id": "1", "yes": "true"})
	registerResetIssuedKey(reg)
	stderrBuf := issuedKeyErrBuffer(t, rctx)

	if err := AppsOpenAPIKeyReset.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdoutBuf.Bytes(), &env); err != nil {
		t.Fatalf("stdout must be a single pure JSON document: %v\n%s", err, stdoutBuf.String())
	}
	if notice, _ := env.Data["notice"].(string); !strings.Contains(notice, shownOnceNotice) {
		t.Errorf("data.notice must carry the one-time guidance, got %q", notice)
	}
	if strings.Contains(stderrBuf.String(), shownOnceNotice) {
		t.Errorf("json mode must not print the notice to stderr:\n%s", stderrBuf.String())
	}
}

// In human (pretty) mode the warning keeps going to stderr and stdout carries
// the readable key block — behavior is unchanged for interactive use.
func TestOpenAPIKeyCreate_Pretty_WarningOnStderr(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		createFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "partner-test"})
	rctx.Format = "pretty"
	registerCreateIssuedKey(reg)
	stderrBuf := issuedKeyErrBuffer(t, rctx)

	if err := AppsOpenAPIKeyCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}

	stderr := stderrBuf.String()
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, shownOnceNotice) {
		t.Errorf("pretty mode must keep the one-time warning on stderr:\n%s", stderr)
	}
	stdout := stdoutBuf.String()
	if !strings.Contains(stdout, "API key:") || !strings.Contains(stdout, "xxxxxxxxxxxx") {
		t.Errorf("pretty mode must print the readable key block to stdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "\"ok\"") {
		t.Errorf("pretty mode must not emit the JSON envelope:\n%s", stdout)
	}
}
