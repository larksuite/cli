// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsAccessScopeGet reads the current access scope configuration of an app.
// 响应原样透传服务端契约（字符串 scope 枚举 All/Tenant/Range + 拆分的 users/departments/chats 数组）。
var AppsAccessScopeGet = common.Shortcut{
	Service:     appsService,
	Command:     "+access-scope-get",
	Description: "Get app access scope configuration",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +access-scope-get --app-id <app_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("%s/apps/%s/access-scope", apiBasePath, validate.EncodePathSegment(appID))).
			Desc("Get app access scope")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := fmt.Sprintf("%s/apps/%s/access-scope", apiBasePath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("GET", path, nil, nil)
		if err != nil {
			return withAppsHint(err, accessScopeFailureHint(err, appIDListHint))
		}
		// 原样透传 — 保留服务端字符串枚举 (All/Tenant/Range)，不合并 users/departments/chats。
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "scope: %v\n", data["scope"])
		})
		return nil
	},
}

// accessScopeUnpublishedMessage is the server's whole message for an access-scope
// call on an app that has never been published (code 3340002, HTTP 409),
// normalized by normalizedExactMessage.
const accessScopeUnpublishedMessage = "application has not been published yet. access scope operations require the app to be published first"

// accessScopeFailureHint picks the hint for an access-scope failure shared by
// +access-scope-get and +access-scope-set, and returns fallback for failures the
// two state codes below do not cover.
//
// 40002 and 3340002 are read together with their message: 3340002 is 40002 plus
// the gateway's 3300000 offset, and the same numbers mean "unsupported app type"
// on other endpoints (see annotateAppLookupErr). Only the two wordings observed
// here — not published yet, creative-mode app — get a specific hint; any other
// message under these codes stops without guessing a cause, so an unknown state
// is never read as permission to publish.
func accessScopeFailureHint(err error, fallback string) string {
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeUnknown {
		return fallback
	}
	if p.Code != 40002 && p.Code != 3340002 {
		return fallback
	}
	switch message := normalizedExactMessage(p.Message); {
	case p.Code == 3340002 && message == accessScopeUnpublishedMessage:
		return "this access-scope operation requires a published app. Publishing takes the whole app live, so get explicit user confirmation first; only then read `lark-cli apps +release-create --help`. Do not publish automatically"
	case message == "创意模式应用不支持查询可用范围，请前往妙搭后台操作", message == "创意模式应用不支持设置可用范围，请前往妙搭后台操作":
		return "creative-mode apps do not support reading or changing access scope through the CLI; use the Miaoda web console and do not retry this command"
	default:
		return "the access-scope operation failed, and the CLI cannot determine a safe recovery step from this response; keep the original error, stop, and review it in the web console or report the request identifier"
	}
}
