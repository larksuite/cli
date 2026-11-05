// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
)

func TestHTMLPublishRecoveryHints(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "html_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "html_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
	for _, tc := range []struct {
		name, subtype, hint string
		setup               func(*testing.T, string)
	}{
		{"Sensitive_Files", "invalid_argument", "remove the listed credential files and their references", func(t *testing.T, dir string) {
			writeRecoveryFile(t, dir, "page.html", `<img src="credentials">`)
			writeRecoveryFile(t, dir, "credentials", "dummy-fixture")
		}},
		{"Entry_Conflict", "failed_precondition", "rename the conflicting non-entry index.html", func(t *testing.T, dir string) {
			writeRecoveryFile(t, dir, "page.html", `<img src="index.html">`)
			writeRecoveryFile(t, dir, "index.html", "<p>other</p>")
		}},
		{"Dependency_Count", "failed_precondition", "reduce the number or nesting depth", func(t *testing.T, dir string) {
			var entry strings.Builder
			for i := 0; i < 201; i++ {
				name := fmt.Sprintf("image%d.png", i)
				fmt.Fprintf(&entry, `<img src="%s">`, name)
				writeRecoveryFile(t, dir, name, "x")
			}
			writeRecoveryFile(t, dir, "page.html", entry.String())
		}},
		{"Dependency_Depth", "failed_precondition", "reduce the number or nesting depth", func(t *testing.T, dir string) {
			writeRecoveryFile(t, dir, "page.html", `<link rel="stylesheet" href="0.css">`)
			for i := 0; i < 18; i++ {
				content := "body{}"
				if i < 17 {
					content = fmt.Sprintf(`@import "%d.css";`, i+1)
				}
				writeRecoveryFile(t, dir, fmt.Sprintf("%d.css", i), content)
			}
		}},
		{"Escaping_Reference_Unchanged", "failed_precondition", "move the file into that directory and rewrite the reference", func(t *testing.T, dir string) {
			writeRecoveryFile(t, dir, "page.html", `<img src="../outside.png">`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"html", "+publish", "--file-path", "page.html", "--dry-run"},
				DefaultAs: "user", WorkDir: dir,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 2)
			assert.Empty(t, result.Stdout)
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Type    string `json:"type"`
					Subtype string `json:"subtype"`
					Hint    string `json:"hint"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(result.Stderr), &envelope))
			assert.False(t, envelope.OK)
			assert.Equal(t, "validation", envelope.Error.Type)
			assert.Equal(t, tc.subtype, envelope.Error.Subtype)
			assert.Contains(t, envelope.Error.Hint, tc.hint)
			for _, flag := range []string{"--allow-sensitive", "--dir", "--entry-file"} {
				assert.NotContains(t, envelope.Error.Hint, flag)
			}
		})
	}
}

func writeRecoveryFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0600))
}
