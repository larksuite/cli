// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dashboardlayout

import "math"

// 读链路的坐标换算。服务端存的是像素，读接口不接受任何栅格参数：
//   - standard_position：像素原值、不取整，是 48 列坐标唯一可靠的来源；
//   - position：服务端按 12 列栅格取整后的值（x/w 按 1494/12 = 124.5px 一列），
//     w=18 这种不是 4 的倍数的 48 列宽度读回来会被取整成 5（×4 = 20），精度已经丢了。
//
// 所以 48 列坐标一律由本包从像素换算，12 列取整值只在服务端还没返回像素时兜底。

// LegacyGridCols 是读接口 position 字段使用的旧栅格列数。
const LegacyGridCols = 12

// LegacyGridScale 是旧栅格一格对应的 48 列格数，用于没有像素坐标时的有损回落。
const LegacyGridScale = GridCols / LegacyGridCols

// GridFromPixels 把像素坐标换算成 48 列栅格值。
//
// 48 列写入的像素本来就是整格倍数（x = 格数 × ColWidthPx），四舍五入只是吸收
// 服务端存储与 JSON 往返带来的浮点误差，不是在做有损取整。
func GridFromPixels(x, y, w, h float64) (gx, gy, gw, gh int) {
	return int(math.Round(x / ColWidthPx)),
		int(math.Round(y / RowHeightPx)),
		int(math.Round(w / ColWidthPx)),
		int(math.Round(h / RowHeightPx))
}

// GridFromLegacy 把旧 12 列取整坐标放大成 48 列坐标。结果只是近似值：
// 取整丢掉的精度补不回来，调用方必须把「坐标有损」告诉使用者。
func GridFromLegacy(x, y, w, h int) (gx, gy, gw, gh int) {
	return x * LegacyGridScale, y * LegacyGridScale, w * LegacyGridScale, h * LegacyGridScale
}
