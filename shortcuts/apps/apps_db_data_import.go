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
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/shortcuts/common"
)

const dbDataImportMaxBytes = 1 * 1024 * 1024 // 1 MB

const dbDataImportHint = "verify --app-id and --table; data file must be .csv/.json and ≤1 MB — split larger files and import in batches"

// AppsDBDataImport 把本地 csv/json 文件直传到应用数据表（high-risk-write）。
//
// POST /apps/{app_id}/db/data_import，multipart 表单：file_name + 可选 table + 文件本体（与
// +file-upload / UploadFileForOpenAPI 一致）。文件的格式解析与转换在服务端 integration 层完成
// （按 file_name 扩展名推断 csv/json），CLI 不再本地解析。表名缺省取文件名（去扩展名）。上限 1 MB。
var AppsDBDataImport = common.Shortcut{
	Service:     appsService,
	Command:     "+db-data-import",
	Description: "Import rows from a local csv/json file into a Miaoda app table",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-data-import --app-id <app_id> --file ./orders.csv --yes",
		"Table defaults to the file name; override with --table.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "file", Desc: "local data file (.csv/.json), relative to cwd", Required: true},
		{Name: "table", Desc: "target table (default: file name without extension)"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("file")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file is required").WithParam("--file")
		}
		// 文件名即可校验格式（服务端按扩展名推断）与推断表名，无需读取内容。
		if _, err := resolveDataFormat(filepath.Ext(rctx.Str("file")), false); err != nil {
			return err
		}
		// 体积守卫前移到 Validate：用 Stat 先查大小（不读内容），dry-run 也能拦超大文件、且
		// 在读整个文件进内存之前就失败（对齐 +file-upload）。Stat 失败不在此报错，留给 Execute
		// 的 ReadInputFile 产出更精确的「文件不存在/越界」错误。
		if st, serr := rctx.FileIO().Stat(strings.TrimSpace(rctx.Str("file"))); serr == nil && st.Size() > dbDataImportMaxBytes {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "import data exceeds 1 MB limit (file is %d bytes); split into ≤1 MB chunks", st.Size()).WithParam("--file")
		}
		if importTableName(rctx) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot infer target table from file name; specify --table").WithParam("--table")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		fileName := filepath.Base(strings.TrimSpace(rctx.Str("file")))
		return common.NewDryRunAPI().
			POST(appDataImportPath(appID)).
			Desc("Import data file into Miaoda app table (multipart upload)").
			Params(dbEnvParams(rctx, map[string]interface{}{"table": importTableName(rctx)})).
			Body(map[string]interface{}{"file_name": fileName, "file": "<contents of --file>"})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		file := strings.TrimSpace(rctx.Str("file"))
		content, err := cmdutil.ReadInputFile(rctx.FileIO(), file)
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file: %v", err).WithParam("--file")
		}
		if len(content) > dbDataImportMaxBytes {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "import data exceeds 1 MB limit (file is %d bytes); split into ≤1 MB chunks", len(content)).WithParam("--file")
		}
		fileName := filepath.Base(file)
		table := importTableName(rctx)

		// multipart：file_name 走表单字段、文件本体走 form-files；env / table 走 query。
		fd := larkcore.NewFormdata()
		fd.AddField("file_name", fileName)
		fd.AddFile("file", bytes.NewReader(content))

		importQuery := larkcore.QueryParams{"table": []string{table}}
		if env := dbEnv(rctx); env != "" {
			importQuery["env"] = []string{env}
		}
		resp, err := rctx.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodPost,
			ApiPath:     appDataImportPath(appID),
			QueryParams: importQuery,
			Body:        fd,
		}, larkcore.WithFileUpload())
		if err != nil {
			return withAppsHint(errs.NewNetworkError(errs.SubtypeNetworkTransport, "import request failed").WithCause(err).WithRetryable(), dbDataImportHint)
		}
		data, err := rctx.ClassifyAPIResponse(resp)
		if err != nil {
			return withAppsHint(err, dbDataImportHint)
		}

		outTable := common.GetString(data, "table")
		if outTable == "" {
			outTable = table
		}
		rows := int64(0)
		if f, ok := numericAsFloat(data["rows"]); ok {
			rows = int64(f)
		}
		out := map[string]interface{}{"file": file, "table": outTable, "rows": rows}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Imported %s → table '%s' (%d rows)\n", file, outTable, rows)
		})
		return nil
	},
}

// importTableName 取目标表名：--table 优先，否则文件名去扩展名。
func importTableName(rctx *common.RuntimeContext) string {
	if t := strings.TrimSpace(rctx.Str("table")); t != "" {
		return t
	}
	f := strings.TrimSpace(rctx.Str("file"))
	if f == "" {
		return ""
	}
	base := filepath.Base(f)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
