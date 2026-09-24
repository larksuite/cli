// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"github.com/larksuite/cli/shortcuts/common"
)

// formRefFlags 返回定位单个表单所需的三个公共 flag。表单子资源命令
// (share / query-page / display-settings / share-qrcode) 全部以这三个值寻址，
// 每次调用都返回新切片，避免多个 Shortcut 共享同一底层数组。
func formRefFlags() []common.Flag {
	return []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
	}
}

// formSubPath 拼接表单子资源的实际请求路径。
func formSubPath(runtime *common.RuntimeContext, segment string) string {
	return baseV3Path(
		"bases", runtime.Str("base-token"),
		"tables", runtime.Str("table-id"),
		"forms", runtime.Str("form-id"),
		segment,
	)
}

// formSubDryRunPath 返回带 :placeholder 的 dry-run 路径模板，占位符与
// setFormRefDryRun 写入的键一一对应；两者必须同时修改，否则 dry-run 输出
// 会留下未解析的占位符。
func formSubDryRunPath(segment string) string {
	return baseV3ServicePath + "/bases/:base_token/tables/:table_id/forms/:form_id/" + segment
}

// setFormRefDryRun 把三个寻址参数写进 dry-run 上下文，供占位符解析。
func setFormRefDryRun(api *common.DryRunAPI, runtime *common.RuntimeContext) *common.DryRunAPI {
	return api.
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", runtime.Str("table-id")).
		Set("form_id", runtime.Str("form-id"))
}
