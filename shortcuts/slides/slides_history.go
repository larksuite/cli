// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type slidesHistoryListSpec struct {
	PageSize  int
	PageToken string
}

type slidesHistoryRevertSpec struct {
	HistoryVersionID string
}

type slidesHistoryRevertStatusSpec struct {
	TaskID string
}

func parseSlidesHistoryPresentation(runtime *common.RuntimeContext) (presentationRef, error) {
	ref, err := parsePresentationRef(runtime.Str("presentation"))
	if err != nil {
		return presentationRef{}, err
	}
	if ref.Kind == "wiki" {
		if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
			return presentationRef{}, err
		}
	}
	return ref, nil
}

func validateSlidesHistoryPageSize(pageSize int) error {
	if pageSize < 1 || pageSize > 20 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --page-size %d: must be between 1 and 20", pageSize).WithParam("--page-size")
	}
	return nil
}

func validateSlidesHistoryVersionID(historyVersionID string) error {
	version, err := strconv.ParseInt(strings.TrimSpace(historyVersionID), 10, 64)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--history-version-id must be a positive integer string returned by slides +history-list").WithParam("--history-version-id").WithCause(err)
	}
	if version <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--history-version-id must be a positive integer string returned by slides +history-list").WithParam("--history-version-id")
	}
	return nil
}

func slidesHistoryListParams(spec slidesHistoryListSpec) map[string]interface{} {
	params := map[string]interface{}{
		"page_size": spec.PageSize,
	}
	if spec.PageToken != "" {
		params["page_token"] = spec.PageToken
	}
	return params
}

func slidesHistoryRevertBody(spec slidesHistoryRevertSpec) map[string]interface{} {
	return map[string]interface{}{
		"history_version_id": spec.HistoryVersionID,
	}
}

func slidesHistoryStatusParams(spec slidesHistoryRevertStatusSpec) map[string]interface{} {
	return map[string]interface{}{
		"task_id": spec.TaskID,
	}
}

func slidesHistoryAPIPath(presentationID, suffix string) string {
	return fmt.Sprintf("/open-apis/slides_ai/v1/xml_presentations/%s/%s", validate.EncodePathSegment(presentationID), suffix)
}

func newSlidesHistoryDryRun(ref presentationRef, desc string) (*common.DryRunAPI, string) {
	dry := common.NewDryRunAPI()
	presentationID := ref.Token
	if ref.Kind == "wiki" {
		presentationID = "<resolved_slides_token>"
		dry.Desc("2-step orchestration: resolve wiki then " + desc).
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to slides presentation").
			Params(map[string]interface{}{"token": ref.Token})
	} else {
		dry.Desc("OpenAPI: " + desc)
	}
	return dry, presentationID
}

// SlidesHistoryList lists history versions of a Slides XML presentation.
var SlidesHistoryList = common.Shortcut{
	Service:           "slides",
	Command:           "+history-list",
	Description:       "List Slides presentation history versions",
	Risk:              "read",
	Scopes:            []string{"slides:presentation:read"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "page-size", Type: "int", Default: "20", Desc: "history entries to return, range 1-20"},
		{Name: "page-token", Desc: "pagination token from the previous page's page_token"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseSlidesHistoryPresentation(runtime); err != nil {
			return err
		}
		return validateSlidesHistoryPageSize(runtime.Int("page-size"))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		spec := slidesHistoryListSpec{
			PageSize:  runtime.Int("page-size"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
		}
		dry, presentationID := newSlidesHistoryDryRun(ref, "list Slides history versions")
		return dry.
			GET(slidesHistoryAPIPath(presentationID, "histories")).
			Params(slidesHistoryListParams(spec)).
			Set("xml_presentation_id", presentationID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}
		spec := slidesHistoryListSpec{
			PageSize:  runtime.Int("page-size"),
			PageToken: strings.TrimSpace(runtime.Str("page-token")),
		}

		data, err := runtime.CallAPITyped(
			http.MethodGet,
			slidesHistoryAPIPath(presentationID, "histories"),
			slidesHistoryListParams(spec),
			nil,
		)
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}

// SlidesHistoryRevert reverts a Slides XML presentation to a history version.
var SlidesHistoryRevert = common.Shortcut{
	Service:           "slides",
	Command:           "+history-revert",
	Description:       "Revert a Slides presentation to a historical version",
	Risk:              "write",
	Scopes:            []string{"slides:presentation:update", "slides:presentation:write_only"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "history-version-id", Desc: "history_version_id from slides +history-list to revert to", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseSlidesHistoryPresentation(runtime); err != nil {
			return err
		}
		if err := validateSlidesHistoryVersionID(runtime.Str("history-version-id")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		spec := slidesHistoryRevertSpec{
			HistoryVersionID: strings.TrimSpace(runtime.Str("history-version-id")),
		}
		dry, presentationID := newSlidesHistoryDryRun(ref, "revert Slides history")
		return dry.
			POST(slidesHistoryAPIPath(presentationID, "history/revert")).
			Body(slidesHistoryRevertBody(spec)).
			Set("xml_presentation_id", presentationID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}
		spec := slidesHistoryRevertSpec{
			HistoryVersionID: strings.TrimSpace(runtime.Str("history-version-id")),
		}

		data, err := runtime.CallAPITyped(
			http.MethodPost,
			slidesHistoryAPIPath(presentationID, "history/revert"),
			nil,
			slidesHistoryRevertBody(spec),
		)
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}

// SlidesHistoryRevertStatus gets the status of a Slides history revert task.
var SlidesHistoryRevertStatus = common.Shortcut{
	Service:           "slides",
	Command:           "+history-revert-status",
	Description:       "Get Slides history revert task status",
	Risk:              "read",
	Scopes:            []string{"slides:presentation:read"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "task-id", Desc: "task_id returned by slides +history-revert", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parseSlidesHistoryPresentation(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("task-id")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		spec := slidesHistoryRevertStatusSpec{
			TaskID: strings.TrimSpace(runtime.Str("task-id")),
		}
		dry, presentationID := newSlidesHistoryDryRun(ref, "get Slides history revert status")
		return dry.
			GET(slidesHistoryAPIPath(presentationID, "history/revert_status")).
			Params(slidesHistoryStatusParams(spec)).
			Set("xml_presentation_id", presentationID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}
		spec := slidesHistoryRevertStatusSpec{
			TaskID: strings.TrimSpace(runtime.Str("task-id")),
		}

		data, err := runtime.CallAPITyped(
			http.MethodGet,
			slidesHistoryAPIPath(presentationID, "history/revert_status"),
			slidesHistoryStatusParams(spec),
			nil,
		)
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}
