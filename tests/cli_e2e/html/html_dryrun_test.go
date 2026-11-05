// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "html_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "html_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("<html>Hello</html>"), 0600))
	for _, tc := range []struct {
		name         string
		args         []string
		path, detail string
	}{
		{"Publish", []string{"html", "+publish", "--file-path", "page.html", "--dry-run"}, "/open-apis/html/v1/publish", "index.html"},
		{"Fetch", []string{"html", "+fetch", "--token", "html_test", "--output", "downloads/source.zip", "--dry-run"}, "/open-apis/html/v1/html_test/download", "downloads/source.zip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: tc.args, DefaultAs: "user", WorkDir: dir})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			assert.Contains(t, result.Stdout, tc.path)
			assert.Contains(t, result.Stdout, tc.detail)
		})
	}
}
