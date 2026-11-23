// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// Source listing accepts 1..100 entries; omitting page_size uses the server's 50.
const maxPageSourceListPageSize = 100
const pageSourceListPath = apiBasePath + "/apps/source/files"

// AppsPageSourceList lists one directory of a shared creative project's source tree.
var AppsPageSourceList = common.Shortcut{
	Service:     appsService,
	Command:     "+page-source-list",
	Description: "List source files and directories of a shared creative app",
	Risk:        "read",
	Scopes:      []string{pageSourceReadScope},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Tips: []string{
		"Requires view permission on the creative page. Lists one directory without recursion or file contents.",
		"Example: lark-cli apps +page-source-list --meta-token <token> --path src --format pretty",
		"Example (next page): lark-cli apps +page-source-list --meta-token <token> --path src --page-token <next_page_token>",
		"Use +page-source-get to read project source; +file-list lists uploaded storage assets.",
	},
	Flags: []common.Flag{
		{Name: "meta-token", Required: true, Desc: "bare share token: last segment of a creative /page/<token> link"},
		{Name: "path", Desc: "relative project directory (default: project root)"},
		{Name: "page-token", Desc: "opaque next_page_token from the previous response; pass unchanged"},
		{Name: "page-size", Type: "int", Desc: "page size (1..100; server default: 50)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return validatePageSourceListFlags(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(pageSourceListPath).
			Desc("List one directory of the creative app's source tree").Params(pageSourceListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if err := validatePageSourceListFlags(rctx); err != nil {
			return err
		}
		query := larkcore.QueryParams{}
		for key, value := range pageSourceListParams(rctx) {
			query.Set(key, fmt.Sprint(value))
		}
		resp, err := rctx.DoAPI(&larkcore.ApiReq{HttpMethod: http.MethodGet, ApiPath: pageSourceListPath, QueryParams: query})
		if err != nil {
			return err
		}
		_, err = rctx.ClassifyAPIResponse(resp)
		if err != nil {
			return pageSourceStatusError(resp.StatusCode, err)
		}
		// Decode from the wire into int64, avoiding precision loss through float64.
		var envelope struct{ Data pageSourceListResponse }
		if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid source directory response").WithCause(err)
		}
		listing := envelope.Data
		if listing.Items == nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "source directory response is missing items")
		}
		meta := &output.Meta{Pagination: &output.PaginationMeta{
			Complete: !listing.HasMore, NextToken: listing.NextPageToken,
			Pages: 1, Items: len(listing.Items),
		}}
		rctx.OutFormat(listing, meta, func(w io.Writer) { renderPageSourceList(w, listing) })
		return nil
	},
}

func validatePageSourceListFlags(rctx *common.RuntimeContext) error {
	if err := validatePageSourceFlags(rctx, false); err != nil {
		return err
	}
	if rctx.Changed("page-size") && (rctx.Int("page-size") < 1 || rctx.Int("page-size") > maxPageSourceListPageSize) {
		return appsValidationParamError("--page-size", "--page-size must be between 1 and %d", maxPageSourceListPageSize)
	}
	return nil
}

func pageSourceListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := pageSourceParams(rctx)
	if rctx.Changed("page-size") {
		params["page_size"] = rctx.Int("page-size")
	}
	if value := rctx.Str("page-token"); value != "" {
		params["page_token"] = value
	}
	return params
}

type pageSourceListResponse struct {
	Items []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"items"`
	NextPageToken string `json:"next_page_token"`
	HasMore       bool   `json:"has_more"`
}

func renderPageSourceList(w io.Writer, listing pageSourceListResponse) {
	if len(listing.Items) == 0 {
		fmt.Fprintln(w, "No source files or directories found.")
	} else {
		rows := make([][]string, 0, len(listing.Items))
		for _, item := range listing.Items {
			rows = append(rows, []string{item.Name, item.Path, item.Type, strconv.FormatInt(item.Size, 10)})
		}
		renderAlignedTable(w, []string{"name", "path", "type", "size (bytes)"}, rows)
	}
	if listing.HasMore {
		fmt.Fprintf(w, "More entries: repeat with the same --path and --page-token %q\n", listing.NextPageToken)
	}
}
