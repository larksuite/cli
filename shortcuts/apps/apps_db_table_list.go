// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const dbTableListHint = "verify --app-id is correct; if targeting --environment dev, create it first with `lark-cli apps +db-env-create --app-id <app_id> --environment dev`"

// AppsDBTableList lists tables in an app's database.
//
// GET /apps/{app_id}/tables（cursor 分页），response items[] 含 estimated_row_count /
// size_bytes optional 字段，默认返回，不必额外传 query。
//
// 输出裁剪：server 给每张表回完整 columns[]（与 +db-table-get 同源、内容一致）。CLI 用白名单
// 投影（dbTableListItem）只组装产品要求字段、把 columns[] 折算成 column_count，避免逐表重复列定义
// 放大 token、并与 +db-table-get 职责区分。完整列定义 / 索引 / 约束 / DDL 用 +db-table-get。
//
// pretty 渲染 5 列：name / description / estimated_row_count / size / columns（即 column_count）；
// 列间两空格、列对齐填充、空 description 用 "—" 占位、size 按 KB/MB/GB 友好格式化。
var AppsDBTableList = common.Shortcut{
	Service:     appsService,
	Command:     "+db-table-list",
	Description: "List tables in an app database (cursor pagination)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-table-list --app-id <app_id>",
		"Tip: filter fields with --jq, e.g. -q '.data.items[].name'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "app id", Required: true},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		return rejectLegacyEnvFlag(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appTablesPath(appID)).
			Desc("List app db tables").
			Params(buildDBTableListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appTablesPath(appID), buildDBTableListParams(rctx), nil)
		if err != nil {
			return withAppsHint(err, dbTableListHint)
		}
		// 白名单投影：只把产品要求的字段组装进 dbTableListItem，替换 server 原始 items[]。
		// server 给每张表回完整 columns[]（与 +db-table-get 同源、逐字节一致），在 list 里逐表
		// 重复既放大 token 又与 schema 职责重叠。这里用白名单而非 delete 黑名单 —— server 后续新增
		// 字段不会自动泄漏进 CLI 输出。需要完整列定义 / 索引 / 约束 / DDL 用 +db-table-get。
		items := projectTableListItems(data["items"])
		data["items"] = items
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderTableListPretty(w, items)
		})
		return nil
	},
}

// dbTableListItem 是 +db-table-list 对外输出的「产品要求字段」白名单。
// 改字段在此处增删即可，无需在 Execute 里逐个 delete server 返回的多余字段。
type dbTableListItem struct {
	Name              string      `json:"name"`
	Description       string      `json:"description"`
	EstimatedRowCount interface{} `json:"estimated_row_count,omitempty"`
	SizeBytes         interface{} `json:"size_bytes,omitempty"`
	ColumnCount       int         `json:"column_count"`
}

// projectTableListItems 把 server 原始 items[]（map）投影成白名单 dbTableListItem 切片。
// column_count 由 server 返回的 columns[] 长度派生（随后 columns[] 不再透出）。
func projectTableListItems(raw interface{}) []dbTableListItem {
	arr, _ := raw.([]interface{})
	out := make([]dbTableListItem, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, dbTableListItem{
			Name:              common.GetString(m, "name"),
			Description:       common.GetString(m, "description"),
			EstimatedRowCount: m["estimated_row_count"],
			SizeBytes:         m["size_bytes"],
			ColumnCount:       deriveColumnCount(m),
		})
	}
	return out
}

func buildDBTableListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := dbEnvParams(rctx, map[string]interface{}{
		"page_size": rctx.Int("page-size"),
	})
	if token := strings.TrimSpace(rctx.Str("page-token")); token != "" {
		params["page_token"] = token
	}
	return params
}

// renderTableListPretty 5 列输出，列间两空格、列对齐填充。
//
// 列名：name / description / estimated_row_count / size / columns。
// 空 description 用 "—" 占位；size 由 size_bytes 经 humanBytes 友好格式化；
// columns 列取白名单投影的 column_count。
func renderTableListPretty(w io.Writer, items []dbTableListItem) {
	headers := []string{"name", "description", "estimated_row_count", "size", "columns"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		desc := item.Description
		if desc == "" {
			desc = "—"
		}
		rows = append(rows, []string{
			item.Name,
			desc,
			intString(item.EstimatedRowCount),
			humanBytes(item.SizeBytes),
			fmt.Sprintf("%d", item.ColumnCount),
		})
	}
	renderAlignedTable(w, headers, rows)
}

// renderAlignedTable 输出列对齐表格：列间两空格、列宽按每列最长 cell 填充、
// 不画 `|` 和 `-` 分隔线、不依赖 TTY 着色。
func renderAlignedTable(w io.Writer, headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if dw := displayWidth(cell); dw > widths[i] {
				widths[i] = dw
			}
		}
	}
	writeRow := func(cells []string) {
		for i, cell := range cells {
			if i >= len(widths) {
				continue
			}
			if i > 0 {
				io.WriteString(w, "  ")
			}
			io.WriteString(w, cell)
			if i < len(widths)-1 {
				pad := widths[i] - displayWidth(cell)
				if pad > 0 {
					io.WriteString(w, strings.Repeat(" ", pad))
				}
			}
		}
		io.WriteString(w, "\n")
	}
	writeRow(headers)
	for _, r := range rows {
		writeRow(r)
	}
}

// displayWidth 估算字符串在 monospace 终端下的显示宽度。
// ASCII 占 1 列；CJK / 全角字符占 2 列；其他多字节字符按 rune 数算（保守）。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r < 0x80:
			w++
		case isWide(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F: // Hangul Jamo
	case r >= 0x2E80 && r <= 0x303E: // CJK Radicals / Kangxi
	case r >= 0x3041 && r <= 0x33FF: // Hiragana / Katakana / Bopomofo / CJK Symbols
	case r >= 0x3400 && r <= 0x4DBF: // CJK Extension A
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
	case r >= 0xA000 && r <= 0xA4CF: // Yi
	case r >= 0xAC00 && r <= 0xD7A3: // Hangul Syllables
	case r >= 0xF900 && r <= 0xFAFF: // CJK Compatibility Ideographs
	case r >= 0xFE30 && r <= 0xFE4F: // CJK Compatibility Forms
	case r >= 0xFF00 && r <= 0xFF60: // Fullwidth Forms
	case r >= 0xFFE0 && r <= 0xFFE6: // Fullwidth Signs
	case r >= 0x20000 && r <= 0x2FFFD: // CJK Extension B-F
	case r >= 0x30000 && r <= 0x3FFFD: // CJK Extension G
	default:
		return false
	}
	return true
}

// humanBytes 把 size_bytes 数值转 KB / MB / GB 友好字符串。
// 1 KiB = 1024 B；与 PG / 操作系统约定一致。
func humanBytes(raw interface{}) string {
	n, ok := numericAsFloat(raw)
	if !ok {
		return "—"
	}
	const unit = 1024.0
	switch {
	case n < unit:
		return fmt.Sprintf("%d B", int64(n))
	case n < unit*unit:
		return fmt.Sprintf("%.0f KB", n/unit)
	case n < unit*unit*unit:
		return formatFloat(n/(unit*unit)) + " MB"
	default:
		return formatFloat(n/(unit*unit*unit)) + " GB"
	}
}

// formatFloat 一位小数；整数值省略小数（24 KB 而不是 24.0 KB；1.5 MB 而不是 1 MB）。
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.1f", f)
}

// intString 把 JSON 反序列化进来的 number 转为整数字符串显示（estimated_row_count）。
func intString(raw interface{}) string {
	if n, ok := numericAsFloat(raw); ok {
		return fmt.Sprintf("%d", int64(n))
	}
	return "—"
}

func numericAsFloat(raw interface{}) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		// 服务端有些数值字段（如 recovery diff 的 inserted/deleted 行数）以字符串下发。
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	case nil:
		return 0, false
	}
	return 0, false
}

// deriveColumnCount 从 items[i].columns 数组长度派生 column_count。
func deriveColumnCount(m map[string]interface{}) int {
	cols, ok := m["columns"].([]interface{})
	if !ok {
		return 0
	}
	return len(cols)
}
