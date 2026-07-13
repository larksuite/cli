// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

const dbDataExportMaxRows = 5000
const dbDataExportMaxBytes = 1 * 1024 * 1024 // 1 MB

const dbDataExportHint = "verify --app-id and --table; if too large, filter rows with +db-execute (WHERE/LIMIT) and export smaller subsets"

// AppsDBDataExport 把应用数据表导出到本地文件（csv/json/sql）。
//
// GET /apps/{app_id}/db/data_export，返回原始字节（非 JSON 信封）。
// 行数不随导出文件返回：CLI 原子编排——先查 GetAppTableRecordList 的 total，再导出文件。
// 数据格式由 --output 扩展名推断（默认 csv，缺省输出 <table>.csv）；上限 5000 行 / 1 MB。
var AppsDBDataExport = common.Shortcut{
	Service:     appsService,
	Command:     "+db-data-export",
	Description: "Export rows from a Miaoda app table to a local file (csv/json/sql)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-data-export --app-id <app_id> --table orders --output ./orders.csv",
		"Format follows the --output extension: .csv / .json / .sql (default csv).",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Desc: "source table", Required: true},
		{Name: "output", Desc: "local output path; extension picks format .csv/.json/.sql (default: <table>.csv)"},
		{Name: "limit", Type: "int", Default: "5000", Desc: "max rows to export (1..5000)"},
	}, dbEnvFlags("", []string{"dev", "online"}, "source db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("table")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--table is required").WithParam("--table")
		}
		if n := rctx.Int("limit"); n <= 0 || n > dbDataExportMaxRows {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--limit must be a positive integer ≤ %d", dbDataExportMaxRows).WithParam("--limit")
		}
		if err := rejectOutputTraversal(rctx.Str("output")); err != nil {
			return err
		}
		if _, _, err := exportFormatAndOutput(rctx); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		format, _, _ := exportFormatAndOutput(rctx)
		return common.NewDryRunAPI().
			GET(appDataExportPath(appID)).
			Desc("Export Miaoda app table data (raw bytes)").
			Params(dbEnvParams(rctx, map[string]interface{}{
				"table":  strings.TrimSpace(rctx.Str("table")),
				"format": format, "limit": rctx.Int("limit"),
			}))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		table := strings.TrimSpace(rctx.Str("table"))
		format, out, err := exportFormatAndOutput(rctx)
		if err != nil {
			return err
		}

		// 原子编排第 1 步：先查总行数（records 列表的 total），再导出文件。
		// total 查询失败不阻断导出——回退到按导出文件内容数行。
		total, totalErr := queryExportTotal(rctx, appID, dbEnv(rctx), table)

		exportQuery := larkcore.QueryParams{
			"table":  []string{table},
			"format": []string{format},
			"limit":  []string{strconv.Itoa(rctx.Int("limit"))},
		}
		if env := dbEnv(rctx); env != "" {
			exportQuery["env"] = []string{env}
		}
		resp, err := rctx.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     appDataExportPath(appID),
			QueryParams: exportQuery,
		})
		if err != nil {
			return withAppsHint(errs.NewNetworkError(errs.SubtypeNetworkTransport, "export request failed").WithCause(err).WithRetryable(), dbDataExportHint)
		}
		// 成功是原始字节；业务错误网关以 JSON 信封 {code,msg} 返回（以 '{' 开头）。
		if b := bytes.TrimSpace(resp.RawBody); len(b) > 0 && b[0] == '{' {
			if _, cerr := rctx.ClassifyAPIResponse(resp); cerr != nil {
				return withAppsHint(cerr, dbDataExportHint)
			}
		}
		if resp.StatusCode >= 400 {
			return withAppsHint(errs.NewNetworkError(errs.SubtypeNetworkServer, "export failed: HTTP %d", resp.StatusCode).WithRetryable(), dbDataExportHint)
		}
		body := resp.RawBody
		if len(body) > dbDataExportMaxBytes {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "export exceeds 1 MB limit (%d bytes); filter rows with +db-execute (WHERE/LIMIT) and export smaller subsets", len(body))
		}

		saved, err := rctx.FileIO().Save(out, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: int64(len(body)),
		}, bytes.NewReader(body))
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output: %v", err).WithParam("--output")
		}
		// 行数取自预查的 total（导出最多 limit 行，故取 min）；total 查询失败时按导出内容数行兜底。
		rows := 0
		if totalErr == nil {
			rows = total
			if lim := rctx.Int("limit"); rows > lim {
				rows = lim
			}
		} else {
			rows = countDataRows(body, format)
		}
		resolved, perr := rctx.FileIO().ResolvePath(out)
		if perr != nil || resolved == "" {
			resolved = out
		}
		result := map[string]interface{}{
			"table": table, "output": resolved, "format": format,
			"rows": rows, "size_bytes": saved.Size(),
		}
		rctx.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Exported %s → %s (%d rows)\n", table, resolved, rows)
		})
		return nil
	},
}

// queryExportTotal 调 GetAppTableRecordList（page_size=1）取 total（符合条件的记录总数）。
// 该接口与 +db-data-export 同为 spark:app:read scope，避免导出命令被迫升级到写权限。
func queryExportTotal(rctx *common.RuntimeContext, appID, env, table string) (int, error) {
	params := map[string]interface{}{"page_size": 1}
	if env != "" {
		params["env"] = env
	}
	raw, err := rctx.CallAPITyped("GET", appTableRecordsPath(appID, table), params, nil)
	if err != nil {
		return 0, err
	}
	return totalAsInt(raw["total"]), nil
}

// totalAsInt 把 total 解析成 int，兼容 JSON number 与 i64-as-string 两种 wire 形态。
func totalAsInt(v interface{}) int {
	if f, ok := numericAsFloat(v); ok {
		return int(f)
	}
	if s, ok := v.(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return n
		}
	}
	return 0
}

// exportFormatAndOutput 由 --output 推断数据格式与落盘路径：
// 给了 --output → 取其扩展名定 format（csv/json/sql）；未给 → 默认 csv、输出 <table>.csv。
func exportFormatAndOutput(rctx *common.RuntimeContext) (format, outPath string, err error) {
	table := strings.TrimSpace(rctx.Str("table"))
	out := strings.TrimSpace(rctx.Str("output"))
	if out == "" {
		return "csv", table + ".csv", nil
	}
	f, ferr := resolveDataFormat(filepath.Ext(out), true)
	if ferr != nil {
		return "", "", ferr
	}
	return f, out, nil
}
