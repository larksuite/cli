// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func createWikiNode(t *testing.T, parentT *testing.T, ctx context.Context, spaceID string, data map[string]any) (gjson.Result, *clie2e.Result, error) {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "post", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes"},
		DefaultAs: "bot",
		Data:      data,
	})
	if err != nil || result.ExitCode != 0 {
		return gjson.Result{}, result, err
	}

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)

	nodeToken := node.Get("node_token").String()
	require.NotEmpty(t, nodeToken, "stdout:\n%s", result.Stdout)
	objType := node.Get("obj_type").String()
	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := deleteWikiNodeAndVerify(cleanupCtx, spaceID, nodeToken, objType)
		clie2e.ReportCleanupFailure(parentT, "delete wiki node "+nodeToken, deleteResult, deleteErr)
	})

	return node, result, nil
}

// createWikiNodeUnderAnyHost creates an isolated parent under an existing
// my_library root node. It avoids adding test nodes directly at the root level,
// whose single-layer limit is easy to exhaust when cleanup regresses. If the
// library is empty, it creates one reusable root host and keeps it for future
// test runs.
func createWikiNodeUnderAnyHost(t *testing.T, parentT *testing.T, ctx context.Context, title string) (gjson.Result, gjson.Result) {
	t.Helper()

	hosts := listWikiRootHosts(t, ctx)
	if len(hosts) == 0 {
		hosts = append(hosts, createWikiRootHost(t, ctx))
	}

	var layerLimitResults []string
	for _, host := range hosts {
		spaceID := host.Get("space_id").String()
		hostNodeToken := host.Get("node_token").String()
		if spaceID == "" || hostNodeToken == "" {
			continue
		}
		node, result, err := createWikiNode(t, parentT, ctx, spaceID, map[string]any{
			"node_type":         "origin",
			"obj_type":          "docx",
			"title":             title,
			"parent_node_token": hostNodeToken,
		})
		if err == nil && result.ExitCode == 0 {
			return host, node
		}
		if isWikiLayerLimitResult(result) {
			layerLimitResults = append(layerLimitResults, fmt.Sprintf("host=%s stdout=%s stderr=%s", hostNodeToken, result.Stdout, result.Stderr))
			continue
		}
		require.NoError(t, err)
		require.Failf(t, "create wiki node under host failed", "host=%s exit=%d stdout=%s stderr=%s", hostNodeToken, result.ExitCode, result.Stdout, result.Stderr)
	}
	require.Failf(t, "create wiki node under host failed", "all candidate hosts hit the single-layer node limit:\n%s", strings.Join(layerLimitResults, "\n"))
	return gjson.Result{}, gjson.Result{}
}

func createWikiRootHost(t *testing.T, ctx context.Context) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"api", "post", "/open-apis/wiki/v2/spaces/my_library/nodes"},
		DefaultAs: "bot",
		Data: map[string]any{
			"node_type": "origin",
			"obj_type":  "docx",
			"title":     "lark-cli-e2e-wiki-host",
		},
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	host := gjson.Get(result.Stdout, "data.node")
	require.True(t, host.Exists(), "stdout:\n%s", result.Stdout)
	require.NotEmpty(t, host.Get("space_id").String(), "stdout:\n%s", result.Stdout)
	require.NotEmpty(t, host.Get("node_token").String(), "stdout:\n%s", result.Stdout)
	return host
}

func listWikiRootHosts(t *testing.T, ctx context.Context) []gjson.Result {
	t.Helper()

	var hosts []gjson.Result
	pageToken := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{"page_size": 50}
		if pageToken != "" {
			if _, exists := seenPageTokens[pageToken]; exists {
				t.Fatalf("wiki root host pagination loop detected for page_token %q", pageToken)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/my_library/nodes"},
			DefaultAs: "bot",
			Params:    params,
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)

		parsed := gjson.Parse(listResult.Stdout)
		hosts = append(hosts, parsed.Get("data.items").Array()...)

		pageToken = parsed.Get("data.page_token").String()
		if pageToken == "" || !parsed.Get("data.has_more").Bool() {
			return hosts
		}
	}
}

func isWikiLayerLimitResult(result *clie2e.Result) bool {
	if result == nil {
		return false
	}
	combined := result.Stdout + "\n" + result.Stderr
	return strings.Contains(combined, "131003") ||
		strings.Contains(strings.ToLower(combined), "single-layer nodes")
}

func getWikiNode(t *testing.T, ctx context.Context, nodeToken string) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/get_node"},
		DefaultAs: "bot",
		Params:    map[string]any{"token": nodeToken},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)
	return node
}

func getWikiSpace(t *testing.T, ctx context.Context, spaceID string) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/" + spaceID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	space := gjson.Get(result.Stdout, "data.space")
	require.True(t, space.Exists(), "stdout:\n%s", result.Stdout)
	return space
}

func listWikiSpaces(t *testing.T, ctx context.Context, pageSize int) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces"},
		DefaultAs: "bot",
		Params:    map[string]any{"page_size": pageSize},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	return gjson.Parse(result.Stdout)
}

type wikiNodeInfo struct {
	NodeToken string
	ObjType   string
}

// deleteWikiNodeAndVerify removes a wiki node, then polls get_node until the
// original node token is gone. Wiki cleanup cannot use drive +delete because
// wiki origin nodes need the backing obj_token and parent nodes must delete
// children first.
func deleteWikiNodeAndVerify(ctx context.Context, spaceID, nodeToken, objType string) (*clie2e.Result, error) {
	getResult, getErr := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/get_node"},
		DefaultAs: "bot",
		Params:    map[string]any{"token": nodeToken},
	}, clie2e.RetryOptions{})
	if getErr != nil {
		return getResult, getErr
	}
	if getResult == nil {
		return nil, fmt.Errorf("get wiki node %s before delete returned nil result", nodeToken)
	}
	if getResult.ExitCode != 0 || !wikiAPISuccess(getResult.Stdout) {
		if isWikiNodeDeletedResult(getResult) {
			getResult.ExitCode = 0
			getResult.RunErr = nil
			return getResult, nil
		}
		return getResult, fmt.Errorf("get wiki node %s before delete failed: exit=%d stdout=%s stderr=%s", nodeToken, getResult.ExitCode, getResult.Stdout, getResult.Stderr)
	}

	node := gjson.Get(getResult.Stdout, "data.node")
	originalNodeToken := nodeToken
	if resolvedSpaceID := node.Get("space_id").String(); resolvedSpaceID != "" {
		spaceID = resolvedSpaceID
	}
	if resolvedObjType := node.Get("obj_type").String(); resolvedObjType != "" {
		objType = resolvedObjType
	}
	if objType == "" {
		objType = "docx"
	}

	children, childListResult, childListErr := listWikiNodeChildren(ctx, spaceID, originalNodeToken)
	if childListErr != nil || childListResult == nil || childListResult.ExitCode != 0 {
		return childListResult, childListErr
	}
	for _, child := range children {
		childDeleteResult, childDeleteErr := deleteWikiNodeAndVerify(ctx, spaceID, child.NodeToken, child.ObjType)
		if childDeleteErr != nil || childDeleteResult == nil || (childDeleteResult.ExitCode != 0 && !isWikiNodeDeletedResult(childDeleteResult)) {
			return childDeleteResult, childDeleteErr
		}
	}

	deleteToken := originalNodeToken
	if node.Get("node_type").String() == "origin" {
		if objToken := node.Get("obj_token").String(); objToken != "" {
			deleteToken = objToken
		}
	}

	deleteResult, deleteErr := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"api", "delete", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes/" + deleteToken},
		DefaultAs: "bot",
		Data:      map[string]any{"obj_type": objType},
	}, clie2e.RetryOptions{})
	if deleteErr != nil || deleteResult == nil {
		return deleteResult, deleteErr
	}
	if deleteResult.ExitCode != 0 || !wikiAPISuccess(deleteResult.Stdout) {
		deleted, verifyErr := isWikiNodeDeleted(ctx, originalNodeToken)
		if verifyErr != nil {
			return deleteResult, verifyErr
		}
		if deleted {
			deleteResult.ExitCode = 0
			return deleteResult, nil
		}
		return deleteResult, fmt.Errorf("wiki node %s still exists after delete failed: exit=%d stdout=%s stderr=%s", originalNodeToken, deleteResult.ExitCode, deleteResult.Stdout, deleteResult.Stderr)
	}
	if err := waitWikiNodeDeleted(ctx, originalNodeToken); err != nil {
		return deleteResult, err
	}
	return deleteResult, nil
}

func listWikiNodeChildren(ctx context.Context, spaceID, parentNodeToken string) ([]wikiNodeInfo, *clie2e.Result, error) {
	var children []wikiNodeInfo
	pageToken := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{
			"page_size":         50,
			"parent_node_token": parentNodeToken,
		}
		if pageToken != "" {
			if _, exists := seenPageTokens[pageToken]; exists {
				return children, nil, fmt.Errorf("wiki children pagination loop detected for parent %s page_token %q", parentNodeToken, pageToken)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes"},
			DefaultAs: "bot",
			Params:    params,
		}, clie2e.RetryOptions{})
		if err != nil || result == nil || result.ExitCode != 0 {
			return children, result, err
		}
		if !wikiAPISuccess(result.Stdout) {
			return children, result, fmt.Errorf("list wiki node children for parent %s failed: stdout=%s stderr=%s", parentNodeToken, result.Stdout, result.Stderr)
		}

		parsed := gjson.Parse(result.Stdout)
		for _, item := range parsed.Get("data.items").Array() {
			nodeToken := item.Get("node_token").String()
			if nodeToken == "" {
				continue
			}
			objType := item.Get("obj_type").String()
			if objType == "" {
				objType = "docx"
			}
			children = append(children, wikiNodeInfo{NodeToken: nodeToken, ObjType: objType})
		}

		pageToken = parsed.Get("data.page_token").String()
		if pageToken == "" || !parsed.Get("data.has_more").Bool() {
			return children, result, nil
		}
	}
}

func waitWikiNodeDeleted(ctx context.Context, nodeToken string) error {
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		deleted, err := isWikiNodeDeleted(ctx, nodeToken)
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("wiki node %s still exists after delete", nodeToken)
		case <-ticker.C:
		}
	}
}

func isWikiNodeDeleted(ctx context.Context, nodeToken string) (bool, error) {
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/get_node"},
		DefaultAs: "bot",
		Params:    map[string]any{"token": nodeToken},
	}, clie2e.RetryOptions{})
	if err != nil {
		return false, err
	}
	if result == nil {
		return false, fmt.Errorf("verify wiki node %s after delete returned nil result", nodeToken)
	}
	if result.ExitCode == 0 && wikiAPISuccess(result.Stdout) {
		return false, nil
	}
	if isWikiNodeDeletedResult(result) {
		return true, nil
	}
	return false, fmt.Errorf("verify wiki node %s after delete: exit=%d stdout=%s stderr=%s", nodeToken, result.ExitCode, result.Stdout, result.Stderr)
}

func wikiAPISuccess(stdout string) bool {
	if ok := gjson.Get(stdout, "ok"); ok.Exists() {
		return ok.Bool()
	}
	if code := gjson.Get(stdout, "code"); code.Exists() {
		return code.Int() == 0
	}
	return false
}

func isWikiNodeDeletedResult(result *clie2e.Result) bool {
	if result == nil {
		return false
	}
	if code := gjson.Get(result.Stdout, "error.code"); code.Exists() && code.Int() == 131005 {
		return true
	}
	if code := gjson.Get(result.Stdout, "code"); code.Exists() && code.Int() == 131005 {
		return true
	}
	combined := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	return strings.Contains(combined, "131005") ||
		strings.Contains(combined, "node not found") ||
		strings.Contains(combined, "not found")
}

func findWikiNodeByToken(t *testing.T, ctx context.Context, spaceID string, nodeToken string, parentNodeTokens ...string) gjson.Result {
	t.Helper()

	pageToken := ""
	lastStdout := ""
	seenPageTokens := map[string]struct{}{}
	for {
		params := map[string]any{"page_size": 50}
		if len(parentNodeTokens) > 0 && parentNodeTokens[0] != "" {
			params["parent_node_token"] = parentNodeTokens[0]
		}
		if pageToken != "" {
			if _, exists := seenPageTokens[pageToken]; exists {
				t.Fatalf("wiki list pagination loop detected for page_token %q, last stdout:\n%s", pageToken, lastStdout)
			}
			seenPageTokens[pageToken] = struct{}{}
			params["page_token"] = pageToken
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"api", "get", "/open-apis/wiki/v2/spaces/" + spaceID + "/nodes"},
			DefaultAs: "bot",
			Params:    params,
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		lastStdout = result.Stdout
		parsed := gjson.Parse(result.Stdout)
		node := parsed.Get(`data.items.#(node_token=="` + nodeToken + `")`)
		if node.Exists() {
			return node
		}

		pageToken = parsed.Get("data.page_token").String()
		if pageToken == "" || !parsed.Get("data.has_more").Bool() {
			t.Fatalf("wiki node %q not found in listed pages, last stdout:\n%s", nodeToken, lastStdout)
		}
	}
}
