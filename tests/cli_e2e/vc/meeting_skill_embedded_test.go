// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMeetingSkillIsEmbeddedInBuiltCLI(t *testing.T) {
	repoRoot := vcContractPath(t)
	bin := filepath.Join(t.TempDir(), "lark-cli")

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, ".")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lark-cli: %v\n%s", err, output)
	}

	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "main skill",
			args: []string{"skills", "read", "lark-meeting"},
			want: "name: lark-meeting",
		},
		{
			name: "scene",
			args: []string{"skills", "read", "lark-meeting", "scenes/query-meeting-and-artifacts.md"},
			want: "# 查询会议及其产物",
		},
		{
			name: "reference",
			args: []string{"skills", "read", "lark-meeting", "references/lark-vc-detail.md"},
			want: "# vc +detail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, bin, tt.args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("lark-cli %v: %v\nstdout:\n%s\nstderr:\n%s", tt.args, err, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("lark-cli %v output missing %q:\n%s", tt.args, tt.want, stdout.String())
			}
		})
	}
}
