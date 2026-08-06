// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
	"github.com/larksuite/cli/shortcuts/minutes"
)

func hasPaginationFlags(runtime *common.RuntimeContext) bool {
	return runtime.Bool("full") || strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0
}

// validateFetchTypeFlags applies type-specific rules before or after Wiki
// resolution so unsupported flags are never silently ignored.
func validateFetchTypeFlags(runtime *common.RuntimeContext, fetchType string) error {
	if fetchType == "minutes" && runtime.As() == core.AsBot {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "minutes can only be fetched with user identity").
			WithParam("--as").
			WithHint("rerun with `--as user`")
	}
	if hasPaginationFlags(runtime) && fetchType == "minutes" {
		return common.ValidationErrorf("--full/--page-token/--page-size do not apply to minutes (got %s)", fetchType).WithParam("--full")
	}
	if strings.TrimSpace(runtime.Str("include")) != "" && fetchType != "minutes" {
		return common.ValidationErrorf("--include only applies to minutes (got %s)", fetchType).WithParam("--include")
	}
	return nil
}

// validateFetch is the Validate hook for drive +fetch.
func validateFetch(_ context.Context, runtime *common.RuntimeContext) error {
	if err := common.ExactlyOneTyped(runtime, "url", "token"); err != nil {
		return err
	}
	if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 0, 0, math.MaxInt32); err != nil {
		return err
	}
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return err
	}
	// For Wiki input the resource type is unknown until unwrap, so defer its
	// type-specific checks until execution.
	if in.inputType != "wiki" {
		if err := validateFetchTypeFlags(runtime, in.inputType); err != nil {
			return err
		}
	}
	if runtime.Bool("full") && (strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0) {
		return common.ValidationErrorf("--full cannot be combined with --page-token/--page-size").WithParam("--full")
	}
	if _, err := minutes.ParseIncludes(runtime.Str("include")); err != nil {
		return err
	}
	return nil
}

// PlanFetchDryRun previews API calls without executing them. For Wiki input it
// stops after get_node because dispatch depends on the live obj_type.
func PlanFetchDryRun(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	dry := common.NewDryRunAPI().Set("type", in.inputType).Set("token", common.MaskToken(in.token))

	if in.inputType == "wiki" {
		dry.Desc("2-step: resolve wiki node → dispatch by obj_type").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying resource").
			Params(map[string]interface{}{"token": in.token}).
			Set("note", "dispatched by obj_type from step 1 (doc/docx/sheet/bitable/slides/file/minutes)")
		return dry.Set("embed_max_rows", runtime.Int("embed-max-rows"))
	}

	switch in.inputType {
	case "doc", "docx":
		body := contentread.NewRequest(fetchResourceURL(runtime.Config.Brand, in, in.inputType, in.token, false))
		body.WithBlockID = true
		contentread.ApplyPagination(&body, runtime.Bool("full"), strings.TrimSpace(runtime.Str("page-token")), runtime.Int("page-size"))
		dry.POST(contentread.Path).
			Desc("fetch document as paginated Markdown with block anchors").
			Body(body)
		dry.POST("/open-apis/docs_ai/v1/documents/<token>/fetch").
			Desc("document API fallback (only if the first read path is unavailable)")
	case "sheet", "bitable", "slides", "file":
		body := contentread.NewRequest(fetchResourceURL(runtime.Config.Brand, in, in.inputType, in.token, false))
		contentread.ApplyPagination(&body, runtime.Bool("full"), strings.TrimSpace(runtime.Str("page-token")), runtime.Int("page-size"))
		dry.POST(contentread.Path).
			Desc(fmt.Sprintf("fetch %s as markdown (paginated)", in.inputType)).
			Body(body)
	case "minutes":
		dry.GET(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", in.token)).
			Desc("minutes: fetch metadata").
			GET(fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", in.token)).
			Desc("minutes: fetch summary, chapters, todos, keywords, and optional transcript").
			Set("include", runtime.Str("include"))
		include, _ := minutes.ParseIncludes(runtime.Str("include"))
		if include["note-doc"] {
			dry.GET("/open-apis/vc/v1/notes/{note_id}").
				Desc("minutes: fetch related document tokens when note_id and vc:note:read are available")
		}
	}
	return dry.Set("embed_max_rows", runtime.Int("embed-max-rows"))
}
