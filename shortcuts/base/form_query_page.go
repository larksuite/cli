// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// 查询页是表单的一个可选关联资源：提交者凭提交结果回查自己填过的内容。
// 服务端用一个查询视图 (query_view_id) 承载它，创建过程分 create /
// associate / readback 三步，任何一步失败都会把已建出的 query_view_id
// 回带在 data.error.value 里，供下一次带 ID 恢复。
const formQueryPageSegment = "query-page"

var BaseFormQueryPageGet = common.Shortcut{
	Service:     "base",
	Command:     "+form-query-page-get",
	Description: "Get the query page associated with a form",
	Risk:        "read",
	Scopes:      []string{"base:form:read"},
	AuthTypes:   authTypes(),
	Flags:       formRefFlags(),
	Tips: []string{
		"Run this before +form-query-page-create: configured=false with query_view_id=null means no query page is associated yet, configured=true means one already exists.",
		"When configured=true, report the existing query_view_id instead of creating again; a second create builds a second query page.",
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return setFormRefDryRun(
			common.NewDryRunAPI().GET(formSubDryRunPath(formQueryPageSegment)),
			runtime,
		)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "GET", formSubPath(runtime, formQueryPageSegment), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseFormQueryPageCreate = common.Shortcut{
	Service:     "base",
	Command:     "+form-query-page-create",
	Description: "Create the query page for a form, or restore a half-created one",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: append(formRefFlags(), common.Flag{
		Name: "query-view-id",
		Desc: "existing query view ID to restore the association with; omit to create a new query page",
	}),
	Tips: []string{
		"Workflow: run +form-query-page-get first. configured=true means the form already has a query page — reuse the returned query_view_id and do not create another one.",
		"Without --query-view-id the request body is {} and the server creates a new query page; the response reports reused=false.",
		"If a create attempt was already tried, resend the SAME query_view_id via --query-view-id to restore the association (the response then reports reused=true). Never retry with an empty POST to transparently rebuild — that leaves the earlier query page orphaned.",
		"On failure the recovery payload is merged into the error hint as error.value={\"completed_steps\":...,\"failed_step\":...,\"query_view_id\":...}; parse it from the hint, the error envelope carries no data field. failed_step is one of create, associate, readback.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if runtime.Changed("query-view-id") && strings.TrimSpace(runtime.Str("query-view-id")) == "" {
			return baseFlagErrorf("--query-view-id must not be empty; omit the flag to create a new query page")
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return setFormRefDryRun(
			common.NewDryRunAPI().
				POST(formSubDryRunPath(formQueryPageSegment)).
				Body(buildFormQueryPageCreateBody(runtime)),
			runtime,
		)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "POST", formSubPath(runtime, formQueryPageSegment),
			nil, buildFormQueryPageCreateBody(runtime))
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// buildFormQueryPageCreateBody 组装创建/恢复请求体。不传 --query-view-id 时
// 发送空对象 {} 表示新建；传了就发 {"query_view_id": "..."} 表示用已有
// 视图恢复关联，服务端据此把 reused 置为 true。
func buildFormQueryPageCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if runtime.Changed("query-view-id") {
		body["query_view_id"] = strings.TrimSpace(runtime.Str("query-view-id"))
	}
	return body
}
