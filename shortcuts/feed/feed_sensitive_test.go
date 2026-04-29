// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package feed

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func defaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func TestFeedSensitive_Validate_MissingEnableDisable(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--user-ids", "ou_user1",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "--enable") || !strings.Contains(err.Error(), "--disable") {
		t.Errorf("error should mention --enable and --disable, got: %v", err)
	}
}

func TestFeedSensitive_Validate_BothEnableAndDisable(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
		"--disable",
		"--user-ids", "ou_user1",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive, got: %v", err)
	}
}

func TestFeedSensitive_Validate_InvalidFeedCardID(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "invalid_id",
		"--enable",
		"--user-ids", "ou_user1",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "oc_") {
		t.Errorf("error should mention oc_ prefix, got: %v", err)
	}
}

func TestFeedSensitive_Validate_EmptyUserIDs(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, FeedSensitive, []string{
		"+sensitive",
		"--feed-card-id", "oc_abc123",
		"--enable",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --user-ids, got nil")
	}
}

// Ensure the package compiles with context import used
var _ = context.Background
