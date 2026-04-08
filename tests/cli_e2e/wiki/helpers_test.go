// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func wikiJSONPayload(t *testing.T, result *clie2e.Result) string {
	t.Helper()

	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		raw = strings.TrimSpace(result.Stderr)
	}

	start := strings.LastIndex(raw, "\n{")
	if start >= 0 {
		start++
	} else {
		start = strings.Index(raw, "{")
	}
	require.NotEqualf(t, -1, start, "json payload not found:\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)

	payload := raw[start:]
	require.Truef(t, gjson.Valid(payload), "invalid json payload:\n%s", payload)
	return payload
}

func skipIfWikiUnavailable(t *testing.T, result *clie2e.Result, reason string) {
	t.Helper()

	payload := wikiJSONPayload(t, result)
	errType := gjson.Get(payload, "error.type").String()
	if errType == "config" && !runningInCI() {
		t.Skipf("%s: %s", reason, gjson.Get(payload, "error.message").String())
	}
}

func runningInCI() bool {
	return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
}

func testSuffix() string {
	return time.Now().UTC().Format("20060102-150405")
}

func createWikiNode(t *testing.T, ctx context.Context, req clie2e.Request) gjson.Result {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, req)
	require.NoError(t, err)
	if result.ExitCode != 0 {
		skipIfWikiUnavailable(t, result, "requires bot wiki node create capability")
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, 0)

	node := gjson.Get(result.Stdout, "data.node")
	require.True(t, node.Exists(), "stdout:\n%s", result.Stdout)

	return node
}
