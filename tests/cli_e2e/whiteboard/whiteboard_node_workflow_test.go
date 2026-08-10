// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWhiteboardNodeLiveWorkflow(t *testing.T) {
	whiteboardToken := os.Getenv("LARK_WHITEBOARD_E2E_TOKEN")
	if whiteboardToken == "" {
		t.Skip("skipped: LARK_WHITEBOARD_E2E_TOKEN not set")
	}
	clie2e.SkipWithoutUserToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	createdText := "lark-cli whiteboard node create " + suffix
	updatedText := "lark-cli whiteboard node update " + suffix
	nodeID := ""

	parentT.Cleanup(func() {
		if nodeID == "" {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"whiteboard", "+node-delete",
				"--whiteboard-token", whiteboardToken,
				"--node-ids", nodeID,
				"--idempotent-token", "wb-node-cleanup-" + suffix,
			},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(parentT, "delete whiteboard node "+nodeID, result, err)
	})

	createSource := marshalWhiteboardNodeSource(t, map[string]interface{}{
		"id":     "tmpNode",
		"type":   "composite_shape",
		"x":      0,
		"y":      0,
		"width":  260,
		"height": 45,
		"text": map[string]interface{}{
			"text":             createdText,
			"font_weight":      "regular",
			"font_size":        14,
			"horizontal_align": "center",
			"vertical_align":   "mid",
		},
		"style": map[string]interface{}{
			"border_color": "#3370ff",
			"border_width": "narrow",
			"border_style": "solid",
			"fill_color":   "#e8f3ff",
		},
		"composite_shape": map[string]interface{}{"type": "round_rect"},
	})
	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+node-create",
			"--whiteboard-token", whiteboardToken,
			"--source", createSource,
			"--idempotent-token", "wb-node-create-" + suffix,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)
	nodeID = gjson.Get(createResult.Stdout, "data.ids").String()
	require.NotEmpty(t, nodeID, "stdout:\n%s", createResult.Stdout)
	require.NotContains(t, nodeID, ",", "single-node create must return one id; stdout:\n%s", createResult.Stdout)

	updateSource := marshalWhiteboardNodeSource(t, map[string]interface{}{
		"id":   nodeID,
		"type": "composite_shape",
		"text": map[string]interface{}{
			"text":             updatedText,
			"font_weight":      "regular",
			"font_size":        14,
			"horizontal_align": "center",
			"vertical_align":   "mid",
		},
	})
	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+node-update",
			"--whiteboard-token", whiteboardToken,
			"--source", updateSource,
			"--idempotent-token", "wb-node-update-" + suffix,
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)
	require.Equal(t, nodeID, gjson.Get(updateResult.Stdout, "data.ids").String(), updateResult.Stdout)
	require.Equal(t, int64(1), gjson.Get(updateResult.Stdout, "data.count").Int(), updateResult.Stdout)

	var readbackStdout string
	err = clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
		Timeout:  30 * time.Second,
		Interval: 2 * time.Second,
		TimeoutError: func() error {
			return fmt.Errorf("updated whiteboard node %s was not visible in raw export", nodeID)
		},
	}, func() (bool, error) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"whiteboard", "+export",
				"--whiteboard-token", whiteboardToken,
				"--output-type", "raw",
			},
			DefaultAs: "user",
			Format:    "json",
		})
		if runErr != nil {
			return false, runErr
		}
		if result.ExitCode != 0 || !gjson.Get(result.Stdout, "status").Bool() {
			return false, fmt.Errorf("raw export failed: exit=%d stdout=%s stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
		}
		readbackStdout = result.Stdout
		node := whiteboardNodeByID(result.Stdout, nodeID)
		return node.Exists() && node.Get("text.text").String() == updatedText, nil
	})
	require.NoError(t, err, "last raw export:\n%s", readbackStdout)

	deleteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"whiteboard", "+node-delete",
			"--whiteboard-token", whiteboardToken,
			"--node-ids", nodeID,
			"--idempotent-token", "wb-node-delete-" + suffix,
		},
		DefaultAs: "user",
		Yes:       true,
	})
	require.NoError(t, err)
	deleteResult.AssertExitCode(t, 0)
	if deleteResult.ExitCode == 0 {
		nodeID = ""
	}
	deleteResult.AssertStdoutStatus(t, true)
	require.Equal(t, int64(1), gjson.Get(deleteResult.Stdout, "data.count").Int(), deleteResult.Stdout)
}

func marshalWhiteboardNodeSource(t *testing.T, node map[string]interface{}) string {
	t.Helper()

	payload, err := json.Marshal(map[string]interface{}{"nodes": []interface{}{node}})
	require.NoError(t, err)
	return string(payload)
}

func whiteboardNodeByID(stdout, nodeID string) gjson.Result {
	for _, node := range gjson.Get(stdout, "data.nodes").Array() {
		if node.Get("id").String() == nodeID {
			return node
		}
	}
	return gjson.Result{}
}
