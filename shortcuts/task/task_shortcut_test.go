// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func taskTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	suffix := strings.NewReplacer("/", "-", " ", "-", ":", "-", "\t", "-").Replace(t.Name())
	return &core.CliConfig{
		AppID:      "test-app-" + suffix,
		AppSecret:  "test-secret-" + suffix,
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_testuser",
		UserName:   "Test User",
	}
}

func warmTenantToken(t *testing.T, f *cmdutil.Factory, reg *httpmock.Registry) {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/test/v1/warm",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{},
		},
	})

	s := common.Shortcut{
		Service:   "test",
		Command:   "+warm-token",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			_, err := rctx.CallAPI("GET", "/open-apis/test/v1/warm", nil, nil)
			return err
		},
	}

	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+warm-token", "--as", "bot"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("warm tenant token: %v", err)
	}
}

func taskShortcutTestFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	return cmdutil.TestFactory(t, taskTestConfig(t))
}

func runMountedTaskShortcut(t *testing.T, shortcut common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	shortcut.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}
