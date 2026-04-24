// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"errors"
	"strings"
	"testing"
)

// realisticPermError mirrors the error shape seen when the app lacks the
// application:application.app_version:readonly scope — HTTP 400 + OAPI
// business code 99991672 + a grant URL buried in the msg field.
const realisticPermError = `API GET /open-apis/application/v6/applications/cli_XXXXXXXXXXXXXXXX/app_versions?lang=zh_cn&page_size=2 returned 400: {"code":99991672,"msg":"Access denied. One of the following scopes is required: [application:application:self_manage, application:application.app_version:readonly].应用尚未开通所需的应用身份权限：[application:application:self_manage, application:application.app_version:readonly]，点击链接申请并开通任一权限即可：https://open.feishu.cn/app/cli_XXXXXXXXXXXXXXXX/auth?q=application:application:self_manage,application:application.app_version:readonly&op_from=openapi&token_type=tenant","error":{"message":"Refer to the documentation...","log_id":"20260421101203E2A5F141245B6F43B3A6"}}`

func TestDescribeAppMetaErr_PermissionDeniedShort(t *testing.T) {
	got := describeAppMetaErr(errors.New(realisticPermError))
	// Must be short enough to read at a glance.
	if len(got) > 400 {
		t.Errorf("summary too long (%d chars): %q", len(got), got)
	}
	// Must name *why* we skipped (so reader knows this is optional preflight,
	// not a hard failure).
	if !strings.Contains(got, "scope") {
		t.Errorf("summary should mention scope requirement, got: %q", got)
	}
	// Must preserve the actionable grant URL — that's the whole point of the
	// long OAPI message, everything else is noise.
	wantURL := "https://open.feishu.cn/app/cli_XXXXXXXXXXXXXXXX/auth?q=application:application:self_manage,application:application.app_version:readonly&op_from=openapi&token_type=tenant"
	if !strings.Contains(got, wantURL) {
		t.Errorf("summary missing grant URL\ngot:  %q\nwant: %q", got, wantURL)
	}
	// Must NOT echo the full JSON envelope (log_id, troubleshooter, etc).
	for _, noise := range []string{"log_id", `"error":`, "Refer to the documentation"} {
		if strings.Contains(got, noise) {
			t.Errorf("summary leaked noise %q: %q", noise, got)
		}
	}
}

func TestDescribeAppMetaErr_UnknownErrorTruncated(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := describeAppMetaErr(errors.New(long))
	if len(got) > 220 { // 200 + ellipsis + small slack
		t.Errorf("unknown error not truncated, len=%d", len(got))
	}
}

func TestDescribeAppMetaErr_ShortErrorPassesThrough(t *testing.T) {
	got := describeAppMetaErr(errors.New("network unreachable"))
	if got != "network unreachable" {
		t.Errorf("short err should pass through unchanged, got: %q", got)
	}
}

func TestDescribeAppMetaErr_LarkOfficeDomain(t *testing.T) {
	// The lark brand uses open.larksuite.com; make sure we match that too so
	// international users get the same short summary.
	msg := `... grant link: https://open.larksuite.com/app/cli_xyz/auth?q=application:application:self_manage&op_from=openapi&token_type=tenant ...`
	got := describeAppMetaErr(errors.New(msg))
	if !strings.Contains(got, "open.larksuite.com") {
		t.Errorf("want larksuite URL extracted, got: %q", got)
	}
}
