// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// Exercise each mounted caller: a terminal Wiki lookup failure must keep its
// metadata, stop before downstream operations, and never be retried.
func TestDriveWikiLookupTerminalErrors(t *testing.T) {
	const wikiURL = "https://example.feishu.cn/wiki/wikiLookupToken"
	const content = `[{"type":"text","text":"test reply"}]`
	cases := []struct {
		shortcut common.Shortcut
		args     []string
	}{
		{DriveInspect, []string{"--url", wikiURL}},
		{DriveCopy, []string{"--url", wikiURL, "--name", "Copy", "--folder-token", "folderTarget"}},
		{DriveExport, []string{"--url", wikiURL, "--file-extension", "pdf"}},
		{DriveExport, []string{"--url", wikiURL, "--file-extension", "markdown"}},
		{DriveAddComment, []string{"--doc", wikiURL, "--content", content}},
		{DriveListComments, []string{"--url", wikiURL}},
		{DriveBatchQueryComments, []string{"--url", wikiURL, "--comment-ids", "commentID"}},
		{DriveResolveComment, []string{"--url", wikiURL, "--comment-id", "commentID"}},
		{DriveRestoreComment, []string{"--url", wikiURL, "--comment-id", "commentID"}},
		{DriveListReplies, []string{"--url", wikiURL, "--comment-id", "commentID"}},
		{DriveAddReply, []string{"--url", wikiURL, "--comment-id", "commentID", "--content", content}},
		{DriveUpdateReply, []string{"--url", wikiURL, "--comment-id", "commentID", "--reply-id", "replyID", "--content", content}},
		{DriveDeleteReply, []string{"--url", wikiURL, "--comment-id", "commentID", "--reply-id", "replyID", "--yes"}},
		{DriveReactReply, []string{"--url", wikiURL, "--reply-id", "replyID", "--emoji", "THUMBSUP", "--action", "add"}},
	}
	for _, tc := range cases {
		for _, identity := range []string{"user", "bot"} {
			for _, failure := range []struct {
				code    int
				subtype errs.Subtype
			}{
				{131012, errs.SubtypeNotFound},
				{131013, errs.SubtypeInvalidParameters},
				{131014, errs.SubtypeFailedPrecondition},
				{131016, errs.SubtypeInvalidParameters},
			} {
				t.Run(fmt.Sprintf("%s/%s/%d", tc.shortcut.Command, identity, failure.code), func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
					lookup := &httpmock.Stub{
						Method: "GET", URL: "/open-apis/wiki/v2/spaces/node_by_token", Reusable: true,
						Headers: http.Header{"X-Tt-Logid": []string{"wiki-lookup-log"}},
						Body:    map[string]interface{}{"code": failure.code, "msg": "lookup rejected"},
						OnMatch: func(req *http.Request) {
							if got := req.URL.RawQuery; got != "token=wikiLookupToken" {
								t.Errorf("lookup query = %q, want token only", got)
							}
						},
					}
					reg.Register(lookup)
					// Match any unexpected downstream request, including a write.
					downstream := &httpmock.Stub{Optional: true, Reusable: true, Body: map[string]interface{}{"code": 1}}
					reg.Register(downstream)
					args := append([]string{tc.shortcut.Command, "--as", identity}, tc.args...)
					err := mountAndRunDrive(t, tc.shortcut, args, f, stdout)
					problem, ok := errs.ProblemOf(err)
					if !ok || problem.Category != errs.CategoryAPI || problem.Subtype != failure.subtype || problem.Code != failure.code || problem.Retryable {
						t.Fatalf("error = %#v (%v), want terminal api/%s/%d", problem, err, failure.subtype, failure.code)
					}
					if problem.LogID != "wiki-lookup-log" || !strings.Contains(problem.Message, "lookup rejected") {
						t.Fatalf("upstream metadata not preserved: %#v", problem)
					}
					if len(lookup.CapturedBodies) != 1 || len(downstream.CapturedBodies) != 0 {
						t.Fatalf("lookup calls = %d, downstream calls = %d", len(lookup.CapturedBodies), len(downstream.CapturedBodies))
					}
					if stdout.Len() != 0 {
						t.Fatalf("unexpected success output: %s", stdout)
					}
				})
			}
		}
	}
}
