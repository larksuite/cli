// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newMeetingShareRuntime() *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{
		"meeting-id", "action", "share-id", "doc-url", "block-id",
		"comment-id", "comment-anchor-id", "element-action", "element-type", "element-block-id",
	} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Float64("block-y-ratio", 0, "")
	cmd.Flags().Bool("comment-focus-shown", false, "")
	cmd.Flags().Bool("comment-panel-open", false, "")
	return common.TestNewRuntimeContext(cmd, defaultConfig())
}

func mustSetMeetingShareFlag(t *testing.T, runtime *common.RuntimeContext, name, value string) {
	t.Helper()
	if err := runtime.Cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Flags().Set(%q, %q) error = %v", name, value, err)
	}
}

func assertMeetingShareValidationError(t *testing.T, err error, wantParam string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v, want validation/invalid_argument", p)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != wantParam {
		t.Fatalf("error = %T %v, want ValidationError param %q", err, err, wantParam)
	}
}

func assertMeetingShareInvalidParams(t *testing.T, err error, wantNames []string) {
	t.Helper()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
	gotNames := make([]string, 0, len(ve.Params))
	for _, param := range ve.Params {
		gotNames = append(gotNames, param.Name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("invalid params = %v, want %v", gotNames, wantNames)
	}
}

func TestMeetingShareContracts(t *testing.T) {
	if got, want := VCMeetingShare.Scopes, []string{"vc:meeting.interaction:write"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("VCMeetingShare.Scopes = %v, want %v", got, want)
	}
	if got := VCMeetingShare.AuthTypes; len(got) != 2 || got[0] != "user" || got[1] != "bot" {
		t.Fatalf("VCMeetingShare.AuthTypes = %v, want [user bot]", got)
	}
	if got, want := VCMeetingShareStatus.Scopes, []string{"vc:meeting.realtime:read"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("VCMeetingShareStatus.Scopes = %v, want %v", got, want)
	}
}

func TestMeetingShareFlagDescriptionsExplainDocumentTypeSupport(t *testing.T) {
	flags := make(map[string]string, len(VCMeetingShare.Flags))
	for _, flag := range VCMeetingShare.Flags {
		flags[flag.Name] = flag.Desc
	}

	if got := flags["doc-url"]; !strings.Contains(got, "Doc, Docx, or Sheet") ||
		!strings.Contains(got, "Wiki") {
		t.Fatalf("--doc-url help = %q", got)
	}
	if got := flags["action"]; !strings.Contains(got, "update_context") ||
		!strings.Contains(got, "Docx/Wiki-Docx only") {
		t.Fatalf("--action help = %q", got)
	}
	if got := flags["block-id"]; !strings.Contains(got, "Docx block ID") {
		t.Fatalf("--block-id help = %q", got)
	}
	if got := flags["comment-focus-shown"]; !strings.Contains(got, "true requires --comment-id and --comment-anchor-id") ||
		!strings.Contains(got, "false preserves the current comment with --comment-id") {
		t.Fatalf("--comment-focus-shown help = %q", got)
	}
	if got := flags["comment-anchor-id"]; !strings.Contains(got, "requires --comment-focus-shown=true") {
		t.Fatalf("--comment-anchor-id help = %q", got)
	}
}

func TestMeetingShareBuildBody(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		want  map[string]interface{}
	}{
		{
			name: "start",
			flags: map[string]string{
				"meeting-id": "7694644829808070175",
				"action":     "start",
				"doc-url":    "https://example.com/docx/docx_token",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "start",
				"share_type": "document",
				"document":   map[string]interface{}{"doc_url": "https://example.com/docx/docx_token"},
			},
		},
		{
			name: "stop",
			flags: map[string]string{
				"meeting-id": "7694644829808070175",
				"action":     "stop",
				"share-id":   "7694862671664909855",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "stop",
				"share_type": "document",
				"share_id":   "7694862671664909855",
			},
		},
		{
			name: "position",
			flags: map[string]string{
				"meeting-id":    "7694644829808070175",
				"action":        "update_context",
				"share-id":      "7694862671664909855",
				"block-id":      "doxbc_position",
				"block-y-ratio": "0.35",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "update_context",
				"share_type": "document",
				"share_id":   "7694862671664909855",
				"document": map[string]interface{}{
					"block_id":      "doxbc_position",
					"block_y_ratio": 0.35,
				},
			},
		},
		{
			name: "comment",
			flags: map[string]string{
				"meeting-id":          "7694644829808070175",
				"action":              "update_context",
				"share-id":            "7694862671664909855",
				"comment-focus-shown": "true",
				"comment-id":          "7380000000000000000",
				"comment-anchor-id":   "doxbc_comment",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "update_context",
				"share_type": "document",
				"share_id":   "7694862671664909855",
				"document": map[string]interface{}{
					"comment_focus_shown": true,
					"comment_id":          "7380000000000000000",
					"comment_anchor_id":   "doxbc_comment",
				},
			},
		},
		{
			name: "comment focus off without comment ID",
			flags: map[string]string{
				"meeting-id":          "7694644829808070175",
				"action":              "update_context",
				"share-id":            "7694862671664909855",
				"comment-focus-shown": "false",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "update_context",
				"share_type": "document",
				"share_id":   "7694862671664909855",
				"document": map[string]interface{}{
					"comment_focus_shown": false,
				},
			},
		},
		{
			name: "comment focus off preserves current comment",
			flags: map[string]string{
				"meeting-id":          "7694644829808070175",
				"action":              "update_context",
				"share-id":            "7694862671664909855",
				"comment-focus-shown": "false",
				"comment-id":          "7380000000000000000",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "update_context",
				"share_type": "document",
				"share_id":   "7694862671664909855",
				"document": map[string]interface{}{
					"comment_focus_shown": false,
					"comment_id":          "7380000000000000000",
				},
			},
		},
		{
			name: "element preview",
			flags: map[string]string{
				"meeting-id":       "7694644829808070175",
				"action":           "update_context",
				"share-id":         "7694862671664909855",
				"element-action":   "open",
				"element-type":     "whiteboard",
				"element-block-id": "doxbc_whiteboard",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "update_context",
				"share_type": "document",
				"share_id":   "7694862671664909855",
				"document": map[string]interface{}{
					"element_preview": map[string]interface{}{
						"action":       "open",
						"element_type": "whiteboard",
						"block_id":     "doxbc_whiteboard",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newMeetingShareRuntime()
			for name, value := range tt.flags {
				mustSetMeetingShareFlag(t, runtime, name, value)
			}
			got, err := buildMeetingShareBody(runtime)
			if err != nil {
				t.Fatalf("buildMeetingShareBody() error = %v", err)
			}
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("body = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestMeetingShareValidation(t *testing.T) {
	tests := []struct {
		name       string
		flags      map[string]string
		wantParam  string
		wantParams []string
	}{
		{
			name:      "reject meeting number",
			flags:     map[string]string{"meeting-id": "618054658", "action": "start", "doc-url": "https://example.com/docx/docx_token"},
			wantParam: "--meeting-id",
		},
		{
			name:      "start requires document URL",
			flags:     map[string]string{"meeting-id": "7694644829808070175", "action": "start"},
			wantParam: "--doc-url",
		},
		{
			name:      "stop requires share id",
			flags:     map[string]string{"meeting-id": "7694644829808070175", "action": "stop"},
			wantParam: "--share-id",
		},
		{
			name: "position and comment are exclusive",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "block-id": "doxbc", "comment-id": "comment",
			},
			wantParams: []string{"--block-id", "--comment-id"},
		},
		{
			name: "comment and element preview are exclusive",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "comment-id": "comment",
				"element-action": "open", "element-type": "image", "element-block-id": "element",
			},
			wantParams: []string{"--comment-id", "--element-action", "--element-type", "--element-block-id"},
		},
		{
			name: "comment anchor requires explicit focus",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "comment-id": "comment", "comment-anchor-id": "doxbc",
			},
			wantParam: "--comment-anchor-id",
		},
		{
			name: "comment focus on requires comment",
			flags: map[string]string{
				"meeting-id":          "7694644829808070175",
				"action":              "update_context",
				"share-id":            "7694862671664909855",
				"comment-focus-shown": "true",
			},
			wantParam: "--comment-id",
		},
		{
			name: "comment focus on requires anchor",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "comment-focus-shown": "true", "comment-id": "comment",
			},
			wantParam: "--comment-anchor-id",
		},
		{
			name: "comment ID requires explicit focus",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "comment-id": "comment",
			},
			wantParam: "--comment-focus-shown",
		},
		{
			name: "comment focus off rejects anchor",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "comment-focus-shown": "false", "comment-id": "comment", "comment-anchor-id": "doxbc",
			},
			wantParam: "--comment-anchor-id",
		},
		{
			name: "ratio range",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "block-id": "doxbc", "block-y-ratio": "1.1",
			},
			wantParam: "--block-y-ratio",
		},
		{
			name: "element needs all fields",
			flags: map[string]string{
				"meeting-id": "7694644829808070175", "action": "update_context",
				"share-id": "7694862671664909855", "element-action": "open",
			},
			wantParam: "--element-type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newMeetingShareRuntime()
			for name, value := range tt.flags {
				mustSetMeetingShareFlag(t, runtime, name, value)
			}
			err := VCMeetingShare.Validate(context.Background(), runtime)
			if len(tt.wantParams) > 0 {
				assertMeetingShareInvalidParams(t, err, tt.wantParams)
				return
			}
			assertMeetingShareValidationError(t, err, tt.wantParam)
		})
	}
}

func TestMeetingShareDryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingShare, []string{
		"+meeting-share", "--dry-run", "--as", "bot",
		"--meeting-id", "7694644829808070175",
		"--action", "start",
		"--doc-url", "https://example.com/docx/docx_token",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"/open-apis/vc/v1/bots/meeting_share",
		`"meeting_id": "7694644829808070175"`,
		`"share_type": "document"`,
		`"doc_url": "https://example.com/docx/docx_token"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("dry-run output missing %q: %s", want, stdout.String())
		}
	}
}

func TestMeetingShareCommentPanelContracts(t *testing.T) {
	var actionFound, flagFound bool
	for _, flag := range VCMeetingShare.Flags {
		if flag.Name == "action" {
			for _, action := range flag.Enum {
				actionFound = actionFound || action == "comment_panel"
			}
		}
		if flag.Name == "comment-panel-open" {
			flagFound = flag.Type == "bool" && strings.Contains(flag.Desc, "Docx/Wiki-Docx only")
		}
	}
	if !actionFound || !flagFound {
		t.Fatalf("comment_panel action = %t, boolean flag with type guidance = %t", actionFound, flagFound)
	}
	for _, shortcut := range []common.Shortcut{VCMeetingShare, VCMeetingShareStatus} {
		if !reflect.DeepEqual(shortcut.AuthTypes, []string{"user", "bot"}) {
			t.Fatalf("%s identities = %v, want user and bot", shortcut.Command, shortcut.AuthTypes)
		}
		for _, internalTerm := range []string{"TAT", "AAT", "UAT", "AE", "Agent Employee"} {
			if strings.Contains(shortcut.Description, internalTerm) {
				t.Fatalf("%s help must not expose %q", shortcut.Command, internalTerm)
			}
		}
	}
}

func TestMeetingShareCommentPanelBodyAndDryRun(t *testing.T) {
	for _, action := range []string{"start", "comment_panel"} {
		for _, value := range []string{"true", "false"} {
			t.Run(action+"/"+value, func(t *testing.T) {
				runtime := newMeetingShareRuntime()
				flags := map[string]string{
					"meeting-id": "7694644829808070175", "action": action,
					"comment-panel-open": value,
				}
				args := []string{
					"+meeting-share", "--dry-run", "--as", "bot",
					"--meeting-id", flags["meeting-id"], "--action", action,
					"--comment-panel-open=" + value,
				}
				want := map[string]interface{}{
					"meeting_id": flags["meeting-id"], "action": action, "share_type": "document",
				}
				document := map[string]interface{}{"comment_panel_open": value == "true"}
				if action == "start" {
					flags["doc-url"] = "https://example.com/wiki/wiki_token"
					document["doc_url"] = flags["doc-url"]
					args = append(args, "--doc-url", flags["doc-url"])
				} else {
					flags["share-id"] = "7694862671664909855"
					want["share_id"] = flags["share-id"]
					args = append(args, "--share-id", flags["share-id"])
				}
				want["document"] = document
				for name, v := range flags {
					mustSetMeetingShareFlag(t, runtime, name, v)
				}
				body, err := buildMeetingShareBody(runtime)
				if err != nil || !reflect.DeepEqual(body, want) {
					t.Fatalf("body = %#v, err = %v, want %#v", body, err, want)
				}
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
				if err := mountAndRun(t, VCMeetingShare, args, f, stdout); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(stdout.String(), `"comment_panel_open": `+value) {
					t.Fatalf("dry-run must preserve explicit %s: %s", value, stdout.String())
				}
			})
		}
	}
}

func TestMeetingShareCommentPanelValidation(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		param string
	}{
		{
			name:  "requires explicit boolean",
			flags: map[string]string{"action": "comment_panel", "share-id": "7694862671664909855"},
			param: "--comment-panel-open",
		},
		{
			name: "rejects document URL",
			flags: map[string]string{
				"action": "comment_panel", "share-id": "7694862671664909855",
				"comment-panel-open": "false", "doc-url": "",
			},
			param: "--doc-url",
		},
	}
	for _, shareID := range []string{"", "0", "-1", "not-an-id", "9223372036854775808"} {
		tests = append(tests, struct {
			name  string
			flags map[string]string
			param string
		}{
			name: "invalid share ID " + shareID,
			flags: map[string]string{
				"action": "comment_panel", "share-id": shareID, "comment-panel-open": "true",
			},
			param: "--share-id",
		})
	}
	for _, action := range []string{"stop", "update_context"} {
		for _, value := range []string{"true", "false"} {
			tests = append(tests, struct {
				name  string
				flags map[string]string
				param string
			}{
				name: action + " rejects " + value,
				flags: map[string]string{
					"action": action, "share-id": "7694862671664909855", "comment-panel-open": value,
				},
				param: "--comment-panel-open",
			})
		}
	}
	for name, value := range map[string]string{
		"block-id": "", "block-y-ratio": "0", "comment-focus-shown": "false",
		"comment-id": "", "comment-anchor-id": "", "element-action": "",
		"element-type": "", "element-block-id": "",
	} {
		tests = append(tests, struct {
			name  string
			flags map[string]string
			param string
		}{
			name: "rejects context " + name,
			flags: map[string]string{
				"action": "comment_panel", "share-id": "7694862671664909855",
				"comment-panel-open": "true", name: value,
			},
			param: "--" + name,
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newMeetingShareRuntime()
			mustSetMeetingShareFlag(t, runtime, "meeting-id", "7694644829808070175")
			for name, value := range tt.flags {
				mustSetMeetingShareFlag(t, runtime, name, value)
			}
			assertMeetingShareValidationError(t, VCMeetingShare.Validate(context.Background(), runtime), tt.param)
		})
	}
}

func TestMeetingShareExecute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/vc/v1/bots/meeting_share",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"share_id": "7694862671664909855"},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingShare, []string{
		"+meeting-share", "--as", "bot", "--format", "json",
		"--meeting-id", "7694644829808070175",
		"--action", "start",
		"--doc-url", "https://example.com/docx/docx_token",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), `"share_id": "7694862671664909855"`) {
		t.Fatalf("output = %s, want share_id", stdout.String())
	}
}

func TestMeetingShareExecuteStopAndUpdate(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]interface{}
	}{
		{
			name: "stop",
			args: []string{
				"+meeting-share", "--as", "bot", "--meeting-id", "7694644829808070175",
				"--action", "stop", "--share-id", "7694862671664909855",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "stop",
				"share_type": "document",
				"share_id":   "7694862671664909855",
			},
		},
		{
			name: "update context",
			args: []string{
				"+meeting-share", "--as", "bot", "--meeting-id", "7694644829808070175",
				"--action", "update_context", "--share-id", "7694862671664909855",
				"--block-id", "doxbc_position", "--block-y-ratio", "0.5",
			},
			want: map[string]interface{}{
				"meeting_id": "7694644829808070175",
				"action":     "update_context",
				"share_type": "document",
				"share_id":   "7694862671664909855",
				"document": map[string]interface{}{
					"block_id":      "doxbc_position",
					"block_y_ratio": 0.5,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/vc/v1/bots/meeting_share",
				Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
			}
			reg.Register(stub)

			if err := mountAndRun(t, VCMeetingShare, tt.args, f, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			reg.Verify(t)

			var got map[string]interface{}
			if err := json.Unmarshal(stub.CapturedBody, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("request body = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMeetingShareStatusDryRunAndExecute(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingShareStatus, []string{
		"+meeting-share-status", "--dry-run", "--as", "user",
		"--meeting-id", "7694644829808070175",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run error: %v", err)
	}
	if out := stdout.String(); !strings.Contains(out, "/open-apis/vc/v1/bots/meeting_share_status") ||
		!strings.Contains(out, `"meeting_id": "7694644829808070175"`) {
		t.Fatalf("dry-run output = %s", out)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/meeting_share_status?meeting_id=7694644829808070175",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"share": map[string]interface{}{"share_id": "7694862671664909855", "title": "Meeting notes"},
			},
		},
	})
	err = mountAndRun(t, VCMeetingShareStatus, []string{
		"+meeting-share-status", "--as", "user", "--format", "json",
		"--meeting-id", "7694644829808070175",
	}, f, stdout)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), `"title": "Meeting notes"`) {
		t.Fatalf("output = %s, want share title", stdout.String())
	}
}

func TestMeetingShareStatusPrettyOutputSanitizesServerFields(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/bots/meeting_share_status?meeting_id=7694644829808070175",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"share": map[string]interface{}{
					"share_id":  "share\x1b[31m",
					"title":     "Meeting\nnotes\u202e",
					"doc_token": "doc\token",
				},
			},
		},
	})
	if err := mountAndRun(t, VCMeetingShareStatus, []string{
		"+meeting-share-status", "--as", "bot", "--format", "pretty",
		"--meeting-id", "7694644829808070175",
	}, f, stdout); err != nil {
		t.Fatal(err)
	}
	reg.Verify(t)
	if got := stdout.String(); strings.Contains(got, "\x1b") ||
		strings.Contains(got, "Meeting\nnotes") ||
		strings.Contains(got, "doc\token") ||
		strings.Contains(got, "\u202e") {
		t.Fatalf("pretty output contains untrusted server content: %q", got)
	}
}
