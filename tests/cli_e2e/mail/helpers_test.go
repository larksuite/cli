// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func mailJSONPayload(t *testing.T, result *clie2e.Result) string {
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
