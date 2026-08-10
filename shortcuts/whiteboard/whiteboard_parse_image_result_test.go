// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func runParseImageResultShortcut(t *testing.T, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "whiteboard"}
	WhiteboardParseImageResult.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func TestWhiteboardParseImageResultDryRun_RequestShape(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)

	err := runParseImageResultShortcut(t, []string{
		"+parse-image-result",
		"--whiteboard-token", "test-board",
		"--task-id", "7670001",
		"--dry-run",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "GET") {
		t.Fatalf("dry-run output should contain GET, got: %s", out)
	}
	if !strings.Contains(out, "/open-apis/board/v1/whiteboards/test...oard/parse_image/7670001") {
		t.Fatalf("dry-run output should contain masked parse_image result URL, got: %s", out)
	}
}

func TestWhiteboardParseImageResultExecute_OutputsSucceededResult(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image/7670001",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id":           "7670001",
				"status":            "succeeded",
				"ids":               []string{"o1:abc", "o1:def"},
				"extra":             map[string]interface{}{"connector": []string{"o1:def"}},
				"previous_revision": "128",
			},
		},
	})

	err := runParseImageResultShortcut(t, []string{
		"+parse-image-result",
		"--whiteboard-token", "test-board",
		"--task-id", "7670001",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeParseImageEnvelope(t, stdout)
	if data["task_id"] != "7670001" || data["status"] != "succeeded" || data["previous_revision"] != "128" {
		t.Fatalf("unexpected output data: %#v", data)
	}
	ids, ok := data["ids"].([]interface{})
	if !ok || len(ids) != 2 {
		t.Fatalf("ids = %#v, want two ids", data["ids"])
	}
}

func TestWhiteboardParseImageResultWaitPollsUntilSucceeded(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	running := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image/7670001",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id": "7670001",
				"status":  "running",
			},
		},
	}
	success := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/board/v1/whiteboards/test-board/parse_image/7670001",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id":           "7670001",
				"status":            "succeeded",
				"ids":               []string{"o1:abc"},
				"previous_revision": "129",
			},
		},
	}
	reg.Register(running)
	reg.Register(success)

	err := runParseImageResultShortcut(t, []string{
		"+parse-image-result",
		"--whiteboard-token", "test-board",
		"--task-id", "7670001",
		"--wait",
		"--timeout", "2s",
		"--interval", "1ms",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeParseImageEnvelope(t, stdout)
	if data["status"] != "succeeded" || data["previous_revision"] != "129" {
		t.Fatalf("unexpected output data: %#v", data)
	}
	if running.CapturedBody == nil || success.CapturedBody == nil {
		t.Fatalf("expected one running poll and one success poll")
	}
}

func TestWhiteboardParseImageResultValidateRejectsBadTaskID(t *testing.T) {
	factory, stdout, _ := parseImageTestFactory(t)
	err := runParseImageResultShortcut(t, []string{
		"+parse-image-result",
		"--whiteboard-token", "test-board",
		"--task-id", "not-a-number",
		"--as", "user",
	}, factory, stdout)
	assertValidationParam(t, err, "--task-id", false)
}

func TestWhiteboardParseImageResultWaitTimeoutReturnsTypedError(t *testing.T) {
	factory, stdout, reg := parseImageTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/board/v1/whiteboards/test-board/parse_image/7670001",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"task_id": "7670001",
				"status":  "running",
			},
		},
	})

	err := runParseImageResultShortcut(t, []string{
		"+parse-image-result",
		"--whiteboard-token", "test-board",
		"--task-id", "7670001",
		"--wait",
		"--timeout", "1ms",
		"--interval", "1ms",
		"--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var networkErr *errs.NetworkError
	if !errors.As(err, &networkErr) {
		t.Fatalf("error type = %T, want *errs.NetworkError", err)
	}
	if networkErr.Subtype != errs.SubtypeNetworkTimeout {
		t.Fatalf("Subtype = %q, want %q", networkErr.Subtype, errs.SubtypeNetworkTimeout)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || !strings.Contains(problem.Hint, "+parse-image-result") {
		t.Fatalf("error hint should contain resumable result command, got problem=%+v err=%v", problem, err)
	}
}

func decodeParseImageResultBody(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode body: %v\nraw=%s", err, string(raw))
	}
	return out
}
