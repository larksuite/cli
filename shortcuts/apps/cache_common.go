// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// 应用运行时缓存（Cache）调试命令共享件：路由 + 环境体 + 渲染。
//
// 三条命令都走 spark OpenAPI `/apps/{app_id}/cache[/clear]`，按运行环境（env→dbBranch）隔离，
// 复用 db 家族的 --environment flag（dbEnvFlags/dbEnv/dbEnvParams/rejectLegacyEnvFlag）：
// get/delete 把 env 放 query（省略即服务端自动选分支），clear 把 env 放 body。

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

// cacheEnvBody 组装 clear 的请求体：仅当显式指定 --environment 才带 env 键；
// 省略时不发（由服务端按应用多环境状态自动选分支），与家族 omit-empty 约定一致。
func cacheEnvBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if env := dbEnv(rctx); env != "" {
		body["env"] = env
	}
	return body
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
