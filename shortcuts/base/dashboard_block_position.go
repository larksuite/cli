// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/dashboardlayout"
	"github.com/larksuite/cli/shortcuts/common"
)

// 读链路与 create / update 回包的坐标口径。
//
// block-get / block-list 两个 GET 接口在 IDL 里只声明了分页参数，查询串里带任何栅格
// 参数都会被网关丢掉，服务端永远按 12 列取整返回 position——早先把 grid_cols=48 放进
// query 发出去，CLI 以为拿到的是 48 列，实际混进了 12 列的数（历史组件的 w/h/y 只有
// 实际的 1/4，整盘被误判成大面积空洞）。所以读请求不再带栅格参数，48 列坐标改由 CLI
// 从服务端返回的 standard_position（像素、不取整）换算。
//
// 输出给调用方的 position 一律改写成 48 列坐标，并删掉 standard_position：模型只看到一套
// 数，不会在 12 列 / 48 列 / 像素三套之间猜该用哪套。

// standardPositionKey 是服务端返回标准参考画布坐标（1494px 宽画布上的像素原值）的字段名。
const standardPositionKey = "standard_position"

// blockPositionKeys 是组件记录里承载栅格坐标的字段名。写入侧只用 position，读回侧
// 历史上也出现过 layout；两个都按同一口径改写，免得漏掉一个仍是 12 列的数。
var blockPositionKeys = []string{"position", "layout"}

// normalizeDashboardBlockPosition 把一条组件记录的坐标原地改写成 48 列口径。
//
//   - 有 standard_position：由像素换算，结果精确；
//   - 没有 standard_position（服务端尚未部署返回像素的版本）：把 12 列取整值 ×4 作为回落，
//     返回 lossy=true，调用方负责在 stderr 说明精度有损，不能静默。
//
// 没有任何可读坐标的记录原样返回，lossy=false。
func normalizeDashboardBlockPosition(block map[string]interface{}) (lossy bool) {
	px, hasPixel := readFloatRect(block[standardPositionKey])
	delete(block, standardPositionKey)

	rewritten := false
	for _, key := range blockPositionKeys {
		pos, ok := block[key].(map[string]interface{})
		if !ok {
			continue
		}
		if hasPixel {
			block[key] = gridRectFromPixels(px)
			rewritten = true
			continue
		}
		x, okX := toIntStrict(pos["x"])
		y, okY := toIntStrict(pos["y"])
		w, okW := toIntStrict(pos["w"])
		h, okH := toIntStrict(pos["h"])
		if !(okX && okY && okW && okH) {
			continue
		}
		gx, gy, gw, gh := dashboardlayout.GridFromLegacy(x, y, w, h)
		block[key] = layoutPositionBody(gx, gy, gw, gh)
		lossy = true
	}
	// 服务端只给了像素、没给 position 时也要让调用方读到 48 列坐标。
	if hasPixel && !rewritten {
		block["position"] = gridRectFromPixels(px)
	}
	return lossy
}

// normalizeDashboardBlockItems 对一页组件列表逐条改写，返回坐标有损的组件 id。
func normalizeDashboardBlockItems(data map[string]interface{}) []string {
	items, _ := data["items"].([]interface{})
	var lossy []string
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if normalizeDashboardBlockPosition(obj) {
			id, _ := obj["block_id"].(string)
			lossy = append(lossy, strings.TrimSpace(id))
		}
	}
	return lossy
}

// normalizeSingleBlockOutput 把 get / create / update 回包里的单个组件坐标改写成 48 列。
//
// 写入侧一直按 48 列提交（fine_grid），但服务端回包里的 position 仍是 12 列取整值：
// 提交 (0,52,48,16) 会读回 (0,13,12,4)，模型会误以为服务端改了它的坐标。三条命令统一
// 走这里，保证调用方读到的坐标与写出去的是同一套栅格。fallbackID 用于回包里没有
// block_id 时的有损提示。
func normalizeSingleBlockOutput(runtime *common.RuntimeContext, data map[string]interface{}, fallbackID string) {
	if data == nil || !normalizeDashboardBlockPosition(data) {
		return
	}
	id, _ := data["block_id"].(string)
	if strings.TrimSpace(id) == "" {
		id = fallbackID
	}
	emitLegacyPositionNotice(runtime, []string{strings.TrimSpace(id)})
}

// gridRectFromPixels 把像素矩形换算成与写入侧同形的 48 列 position 对象。
func gridRectFromPixels(px [4]float64) map[string]interface{} {
	x, y, w, h := dashboardlayout.GridFromPixels(px[0], px[1], px[2], px[3])
	return layoutPositionBody(x, y, w, h)
}

// readFloatRect 读出 {x,y,w,h} 四个数值，缺任何一个都视为没有。
func readFloatRect(v interface{}) ([4]float64, bool) {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return [4]float64{}, false
	}
	var out [4]float64
	for i, key := range positionKeys {
		n, ok := toFloat(obj[key])
		if !ok {
			return [4]float64{}, false
		}
		out[i] = n
	}
	return out, true
}

// toFloat 接受 JSON 解码可能产出的数值类型。
func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case interface{ Float64() (float64, error) }:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// legacyPositionNoticePrefix 是「读到的是 12 列取整坐标」提示的固定开头。
const legacyPositionNoticePrefix = "服务端没有返回 standard_position"

// emitLegacyPositionNotice 在有组件只能按 12 列取整坐标回落时，向 stderr 陈述一次。
// 每次命令最多一行，列出受影响的组件；没有受影响组件时什么都不写。
func emitLegacyPositionNotice(runtime *common.RuntimeContext, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(runtime.IO().ErrOut,
		"%s，组件 %s 的 position 是把服务端 %d 列取整坐标 ×%d 换算出的 %d 列近似值，精度有损：每个坐标可能与实际相差最多 %d 格。\n",
		legacyPositionNoticePrefix, strings.Join(ids, "、"),
		dashboardlayout.LegacyGridCols, dashboardlayout.LegacyGridScale, dashboardlayout.GridCols,
		dashboardlayout.LegacyGridScale/2)
}
