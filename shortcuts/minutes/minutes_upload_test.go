// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func TestMinutesUpload_Validate(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing file token",
			args:    []string{"+upload", "--as", "user"},
			wantErr: "required flag(s) \"file-token\" not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "minutes"}
			MinutesUpload.Mount(parent, f)
			parent.SetArgs(tt.args)
			parent.SilenceErrors = true
			parent.SilenceUsage = true
			err := parent.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got: %s", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMinutesUpload_HelpMetadata(t *testing.T) {
	if len(MinutesUpload.Flags) == 0 {
		t.Fatal("expected file-token flag metadata")
	}
	if got := MinutesUpload.Flags[0].Desc; !strings.Contains(got, "supported audio/video file") {
		t.Fatalf("file-token description = %q, want supported media guidance", got)
	}

	joinedTips := strings.Join(MinutesUpload.Tips, "\n")
	for _, want := range []string{
		"drive +upload",
		"wav, mp3, m4a, aac, ogg, wma, amr",
		"avi, wmv, mov, mp4, m4v, mpeg, ogg, flv",
		"6GB",
		"6 hours",
	} {
		if !strings.Contains(joinedTips, want) {
			t.Fatalf("tips should contain %q, got:\n%s", want, joinedTips)
		}
	}
}

func TestMinutesUpload_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesUpload, []string{"+upload", "--file-token", "boxcn123456", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "POST") || !strings.Contains(out, "/open-apis/minutes/v1/minutes/upload") {
		t.Errorf("expected POST /open-apis/minutes/v1/minutes/upload, got:\n%s", out)
	}
	if !strings.Contains(out, "boxcn123456") {
		t.Errorf("expected file token in body, got:\n%s", out)
	}
}

func TestMinutesUpload_Execute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    "/open-apis/minutes/v1/minutes/upload",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"minute_url": "https://sample.feishu.cn/minutes/obcnq3b9jl72l83w4f149w9c",
			},
		},
	})

	err := mountAndRun(t, MinutesUpload, []string{"+upload", "--file-token", "boxcn123456", "--format", "json", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	dataMap, _ := res["data"].(map[string]interface{})
	if dataMap["minute_url"] != "https://sample.feishu.cn/minutes/obcnq3b9jl72l83w4f149w9c" {
		t.Errorf("expected minute_url https://sample.feishu.cn/minutes/obcnq3b9jl72l83w4f149w9c, got %v", dataMap["minute_url"])
	}
}
