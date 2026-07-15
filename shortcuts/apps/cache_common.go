// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// 应用运行时缓存（Cache）调试命令共享件：路由 + 环境 flag + 渲染。
//
// 三条命令都走 spark OpenAPI `/apps/{app_id}/cache[/clear]`，按运行环境（env→dbBranch）隔离：
// 环境 flag 用 cacheEnvFlag()（只 --environment，不带 db 家族的旧名 --env），env 值经 dbEnv 读、
// 经 dbEnvParams 注入——get/delete 放 query，clear 放 body（省略即服务端自动选分支）。

// appCachePath 返回缓存单 key 读/删 URL：cache（GET 读、DELETE 删，靠方法区分）。
func appCachePath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/cache", apiBasePath, validate.EncodePathSegment(appID))
}

// appCacheClearPath 返回清空指定环境缓存 URL：cache/clear。
func appCacheClearPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/cache/clear", apiBasePath, validate.EncodePathSegment(appID))
}

// cacheEnvFlag 返回缓存命令的运行环境 flag。cache 是全新命令、从无旧名 --env，
// 故只注册干净的 --environment（不带 db 家族那套隐藏 --env + 拒收逻辑）。
// 省略即服务端按应用多环境状态自动选分支（多环境→dev，非多环境→online）。
func cacheEnvFlag() common.Flag {
	return common.Flag{
		Name: "environment",
		Enum: []string{"dev", "online"},
		Desc: "target runtime environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online",
	}
}

// cacheBool 防御性解析布尔：真 bool 直接用；若服务端把 exists 返成字符串 "true"/"false" 也归一成 bool，
// 其它类型按 false。避免 exists 万一以字符串形态出现时被误判成未命中（hit→miss 翻转）。
func cacheBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	}
	return false
}

// cacheInt 把服务端下发的数值字段归一成 int64（无法解析→nil）。本仓惯例：数值可能以字符串下发
// （见 numericAsFloat 的 string 分支），若直接透传，--format json 的字段类型会随服务端 wire 形态漂移
// （number ↔ string）。归一后输出类型恒定为数字或 null，消费方无需自己容忍字符串。
func cacheInt(raw interface{}) interface{} {
	if f, ok := numericAsFloat(raw); ok {
		return int64(f)
	}
	return nil
}

// resolvedEnv 取服务端回吐的 resolved env；缺失时兜底成请求侧 --environment（可能为空）。
// 省略 --environment 时服务端自动选分支，靠服务端回吐才知道实际命中 dev / online。
func resolvedEnv(data map[string]interface{}, rctx *common.RuntimeContext) string {
	if env := common.GetString(data, "env"); env != "" {
		return env
	}
	return dbEnv(rctx)
}

// formatCacheTTL 把剩余 TTL（毫秒）格式化成 4m32s 这样的时长串；非数字返回 "—"。
func formatCacheTTL(ms interface{}) string {
	f, ok := numericAsFloat(ms)
	if !ok {
		return "—"
	}
	return (time.Duration(int64(f)) * time.Millisecond).String()
}

// printCacheValuePretty 把 value 反序列化后缩进展开（pretty 口径）；非 JSON 则原样打印。
// 与「json 原样字符串、pretty 才反序列化」的设计一致。
func printCacheValuePretty(w io.Writer, raw string) {
	v := safeParseJSON(raw)
	if s, ok := v.(string); ok {
		fmt.Fprintln(w, s)
		return
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintln(w, raw)
		return
	}
	w.Write(b)
	fmt.Fprintln(w)
}
