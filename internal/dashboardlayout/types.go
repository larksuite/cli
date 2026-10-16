// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package dashboardlayout 定义仪表盘组件的 48 列栅格口径：栅格常量、组件坐标，
// 以及服务端像素坐标到 48 列栅格值的换算（见 pixels.go）。
//
// 本包是命令无关的纯机制：不做 I/O、不依赖 runtime，坐标一律是 48 列栅格值。
// 布局的整理（空洞、重叠、越界）由服务端的兜底整理负责，本包不做排布与评判。
package dashboardlayout

// 栅格体系常量。画布宽固定 48 格（1494px），高度无上限。
const (
	// GridCols 是画布列数。
	GridCols = 48
	// ColWidthPx 是单列像素宽，48 × 31.125 = 1494px。
	ColWidthPx = 31.125
	// RowHeightPx 是单行像素高。
	RowHeightPx = 26
)

// Component 是一个组件的 48 列栅格坐标与尺寸。
type Component struct {
	// ID 是组件标识（block_id）。
	ID string
	// ChartType 是后端图表类型，仅作描述。
	ChartType string
	X         int
	Y         int
	W         int
	H         int
}
