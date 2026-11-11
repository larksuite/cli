// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const pageSourceGetPath = apiBasePath + "/apps/source/file"

// Match the source API's 10 MiB limit. Buffer stdout before scanning so block
// mode cannot leak a partial file; never trust Content-Length as the bound.
const maxPageSourceFileBytes = 10 * 1024 * 1024

// AppsPageSourceGet streams a shared creative source file verbatim to stdout or FileIO.
var AppsPageSourceGet = common.Shortcut{
	Service:     appsService,
	Command:     "+page-source-get",
	Description: "Read a UTF-8 source file from a shared creative app",
	Risk:        "read",
	Scopes:      []string{pageSourceReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Tips: []string{
		"Requires view permission on the creative page; binary and oversized files are rejected by the server.",
		"Example: lark-cli apps +page-source-get --meta-token <token> --path src/main.ts",
		"Example (save): lark-cli apps +page-source-get --meta-token <token> --path src/main.ts --output ./main.ts",
		"Without --output, stdout is the exact file content regardless of --format; no JSON envelope or extra newline is added. --jq is not supported for raw file output.",
		"With --output, the file is saved through FileIO and stdout reports its path and size in the selected format.",
	},
	Flags: []common.Flag{
		{Name: "meta-token", Required: true, Desc: "bare share token: last segment of a creative /page/<token> link"},
		{Name: "path", Required: true, Desc: "relative project file path; use +page-source-list to discover files"},
		{Name: "output", Desc: "save to a relative path within the current directory (default: write raw content to stdout)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error { return validatePageSourceGetFlags(rctx) },
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(pageSourceGetPath).
			Desc("Read a UTF-8 source file to stdout or --output").Params(pageSourceParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validatePageSourceGetFlags(rctx); err != nil {
			return err
		}
		query := larkcore.QueryParams{}
		query.Set("meta_token", rctx.Str("meta-token"))
		query.Set("path", rctx.Str("path"))
		resp, err := rctx.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet, ApiPath: pageSourceGetPath, QueryParams: query,
		}, client.WithReplaySafe())
		if err != nil {
			if problem, ok := errs.ProblemOf(err); ok {
				return pageSourceStatusError(problem.Code, err)
			}
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return errs.NewAPIError(errs.SubtypeUnknown, "source request returned HTTP %d", resp.StatusCode).WithCode(resp.StatusCode)
		}
		// Any successful body is source, including JSON resembling an API error.
		// Do not run the archive export's JSON-envelope detection on text files.
		out := rctx.Str("output")
		if out == "" {
			content, err := io.ReadAll(io.LimitReader(resp.Body, maxPageSourceFileBytes+1))
			if err != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "could not read source content: %v", err).WithCause(err)
			}
			if len(content) > maxPageSourceFileBytes {
				return errs.NewAPIError(errs.SubtypeFailedPrecondition, "source content exceeds the 10 MiB server limit")
			}
			scan := output.ScanTextForSafety(rctx.CommandPath(), string(content), rctx.IO().ErrOut)
			if scan.Blocked {
				return scan.BlockErr
			}
			if err := output.WriteAlertWarning(rctx.IO().ErrOut, scan.Alert); err != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "could not write content safety warning").WithCause(err)
			}
			if _, err := io.Copy(rctx.IO().Out, bytes.NewReader(content)); err != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "could not stream source content to stdout: %v", err).WithCause(err)
			}
			return nil
		}
		saved, err := rctx.FileIO().Save(out, fileio.SaveOptions{
			ContentType: resp.Header.Get("Content-Type"), ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return common.WrapSaveErrorTypedForFlag(err, "--output")
		}
		resolved, err := rctx.FileIO().ResolvePath(out)
		if err != nil || resolved == "" {
			resolved = out
		}
		rctx.OutFormat(map[string]interface{}{"output": resolved, "size_bytes": saved.Size()}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Saved %s (%d bytes)\n", resolved, saved.Size())
		})
		return nil
	},
}

func validatePageSourceGetFlags(rctx *common.RuntimeContext) error {
	if err := validatePageSourceFlags(rctx, true); err != nil {
		return err
	}
	if err := validatePageSourceText(rctx.Str("output"), "--output"); err != nil {
		return err
	}
	if err := rejectOutputTraversal(rctx.Str("output")); err != nil {
		return err
	}
	if rctx.Str("output") == "" && rctx.JqExpr != "" {
		return appsValidationParamError("--jq", "--jq cannot filter raw source content").
			WithHint("omit --jq to read the original file; filter the resulting file separately if needed")
	}
	return nil
}
