// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/doc/internal/docxparse"
)

const (
	docsCreateBatchTarget         = 2_000
	docsCreateOperationBlockLimit = 5_000
	docsCreateTotalBlockLimit     = 40_000
)

func createBatchLimits() docxparse.CreateBatchLimits {
	return docxparse.CreateBatchLimits{
		TargetBlocks:    docsCreateBatchTarget,
		OperationBlocks: docsCreateOperationBlockLimit,
		TotalBlocks:     docsCreateTotalBlockLimit,
	}
}

type docsCreateWritePlan struct {
	CreateBody   map[string]interface{}
	AppendBodies []map[string]interface{}
	TotalBlocks  int
}

func buildCreateWritePlan(runtime *common.RuntimeContext) (docsCreateWritePlan, []localDocResource, error) {
	body, resources, err := buildCreateBodyWithPreparedInput(runtime)
	if err != nil {
		return docsCreateWritePlan{}, nil, err
	}
	plan := docsCreateWritePlan{CreateBody: body}
	content, _ := body["content"].(string)
	var batchPlan docxparse.CreateBatchPlan
	format := strings.TrimSpace(runtime.Str("doc-format"))
	switch format {
	case "markdown":
		batchPlan, err = docxparse.PlanCreateMarkdownBatchesWithLimits(content, createBatchLimits())
	default:
		batchPlan, err = docxparse.PlanCreateBatchesWithLimits(content, createBatchLimits())
	}
	if err != nil {
		var planErr *docxparse.CreateBatchPlanError
		if !errors.As(err, &planErr) {
			if format == "markdown" {
				return docsCreateWritePlan{}, nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
					"--content cannot be safely partitioned with the DocxXML SDK Markdown parser: %v", err).
					WithParam("--content").
					WithHint("fix the malformed Markdown or split it into smaller files before retrying").
					WithCause(err)
			}
			// Preserve the existing create contract for compatibility-tolerant XML.
			// The service remains the semantic parser when strict local planning is
			// unavailable.
			return plan, resources, nil
		}
		return docsCreateWritePlan{}, nil, docsCreateBatchValidationError(planErr)
	}
	plan.TotalBlocks = batchPlan.TotalBlocks
	if len(batchPlan.Batches) <= 1 {
		return plan, resources, nil
	}
	plan.CreateBody["content"] = batchPlan.Batches[0]
	for _, contentBatch := range batchPlan.Batches[1:] {
		plan.AppendBodies = append(plan.AppendBodies, buildCreateAppendBody(body, contentBatch))
	}
	return plan, resources, nil
}

func docsCreateBatchValidationError(planErr *docxparse.CreateBatchPlanError) error {
	if planErr == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "create batch planner returned an empty error")
	}
	switch planErr.Kind {
	case docxparse.CreateBatchTotalLimit:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content materializes %d blocks, exceeding the document limit %d", planErr.Blocks, planErr.Limit).
			WithParam("--content").
			WithHint("split the content across multiple documents before retrying")
	case docxparse.CreateBatchSubtreeLimit:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"top-level %s unit materializes %d blocks and cannot fit in one append batch (limit %d)", planErr.Tag, planErr.Blocks, planErr.Limit).
			WithParam("--content").
			WithHint("split the oversized top-level container into smaller sibling blocks before retrying")
	case docxparse.CreateBatchInitialCapacity:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"the first top-level %s unit materializes %d blocks and cannot fit in the initial create batch (available %d)", planErr.Tag, planErr.Blocks, planErr.Limit).
			WithParam("--content").
			WithHint("split the first container or place a smaller block before it so the remaining subtree can be appended")
	case docxparse.CreateBatchTitleAfterCreate:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"top-level %s must remain in the initial create batch to preserve SDK create-title analysis", planErr.Tag).
			WithParam("--content").
			WithHint("reduce the content before the title-bearing H1, or pass an explicit --title that does not require removing a matching body H1")
	default:
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "unknown create batch planning error %q", planErr.Kind)
	}
}

func buildCreateAppendBody(createBody map[string]interface{}, content string) map[string]interface{} {
	body := map[string]interface{}{
		"format":      createBody["format"],
		"command":     "block_insert_after",
		"block_id":    "-1",
		"content":     content,
		"revision_id": -1,
	}
	if referenceMap, ok := createBody["reference_map"]; ok {
		body["reference_map"] = referenceMap
	}
	if scene, ok := createBody["scene"]; ok {
		body["scene"] = scene
	}
	return body
}

func appendCreateBatchDryRuns(dry *common.DryRunAPI, plan docsCreateWritePlan) *common.DryRunAPI {
	for index, body := range plan.AppendBodies {
		dry.PUT("/open-apis/docs_ai/v1/documents/<created_document_id>").
			Desc(fmt.Sprintf("OpenAPI: append create batch %d of %d", index+2, len(plan.AppendBodies)+1)).
			Body(body)
	}
	if len(plan.AppendBodies) > 0 {
		dry.Set("create_batch_count", len(plan.AppendBodies)+1).
			Set("create_total_blocks", plan.TotalBlocks)
	}
	return dry
}

func executeCreateAppendBatches(runtime *common.RuntimeContext, plan docsCreateWritePlan, data map[string]interface{}, resources []localDocResource) error {
	if len(plan.AppendBodies) == 0 {
		return nil
	}
	doc := common.GetMap(data, "document")
	documentID := strings.TrimSpace(common.GetString(doc, "document_id"))
	if documentID == "" {
		return finishCreateBatchFailure(runtime, data, resources, "", len(plan.AppendBodies)+1, 1, 2,
			errs.NewInternalError(errs.SubtypeInvalidResponse, "create response is missing document_id; append batches were not attempted"))
	}
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", validate.EncodePathSegment(documentID))
	for index, body := range plan.AppendBodies {
		batchNumber := index + 2
		batchData, err := doDocAPI(runtime, "PUT", apiPath, body)
		if err != nil {
			return finishCreateBatchFailure(runtime, data, resources, documentID, len(plan.AppendBodies)+1, batchNumber-1, batchNumber, err)
		}
		if docsAPIOperationFailed(batchData) {
			mergeCreateBatchWarnings(data, batchData["warnings"])
			return finishCreateBatchFailure(runtime, data, resources, documentID, len(plan.AppendBodies)+1, batchNumber-1, batchNumber,
				errs.NewAPIError(errs.SubtypeUnknown, "append batch %d returned result=failed", batchNumber))
		}
		mergeCreateBatchData(data, batchData)
	}
	return nil
}

func mergeCreateBatchData(createData, batchData map[string]interface{}) {
	if createData == nil || batchData == nil {
		return
	}
	createDoc := common.GetMap(createData, "document")
	batchDoc := common.GetMap(batchData, "document")
	if createDoc != nil && batchDoc != nil {
		if blocks := common.GetSlice(batchDoc, "new_blocks"); len(blocks) > 0 {
			createDoc["new_blocks"] = append(common.GetSlice(createDoc, "new_blocks"), blocks...)
		}
		if revision, ok := batchDoc["revision_id"]; ok {
			createDoc["revision_id"] = revision
		}
	}
	mergeCreateBatchWarnings(createData, batchData["warnings"])
}

func mergeCreateBatchWarnings(data map[string]interface{}, raw interface{}) {
	switch warnings := raw.(type) {
	case []interface{}:
		for _, warning := range warnings {
			appendDocWarning(data, fmt.Sprint(warning))
		}
	case []string:
		for _, warning := range warnings {
			appendDocWarning(data, warning)
		}
	case string:
		appendDocWarning(data, warnings)
	}
}

func finishCreateBatchFailure(runtime *common.RuntimeContext, data map[string]interface{}, resources []localDocResource, documentID string, totalBatches, completedBatches, failedBatch int, cause error) error {
	if data == nil {
		data = map[string]interface{}{}
	}
	detail := map[string]interface{}{
		"total_batches":     totalBatches,
		"completed_batches": completedBatches,
		"failed_batch":      failedBatch,
	}
	if problem, ok := errs.ProblemOf(cause); ok {
		detail["error"] = *problem
	}
	data["create_batches"] = detail
	data["result"] = "partial_success"
	appendDocWarning(data, fmt.Sprintf("document creation completed %d of %d batches; batch %d failed and later batches were not attempted", completedBatches, totalBatches, failedBatch))
	if len(resources) > 0 && documentID != "" {
		if err := finalizeLocalDocResources(runtime, documentID, data, resources); err != nil {
			return err
		}
	}
	return runtime.OutPartialFailure(data, nil)
}
