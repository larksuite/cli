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
		Content:         docxparse.DefaultContentLimits(),
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
	format := strings.TrimSpace(runtime.Str("doc-format"))
	batchPlan, err := planCreateContentBatches(format, content)
	if err != nil {
		if validationErr := docsCreatePlanningValidationError(format, content, err); validationErr != nil {
			return docsCreateWritePlan{}, nil, validationErr
		}
		// Preserve the existing single-create contract for compatibility-tolerant
		// XML when strict source-preserving batch boundaries are unavailable.
		return plan, resources, nil
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

func planCreateContentBatches(format, content string) (docxparse.CreateBatchPlan, error) {
	if strings.TrimSpace(format) == "markdown" {
		return docxparse.PlanCreateMarkdownBatchesWithLimits(content, createBatchLimits())
	}
	return docxparse.PlanCreateBatchesWithLimits(content, createBatchLimits())
}

// validateCreateContentPreflight runs before local resource rewriting so a
// rejected create request cannot read/convert resource payloads or write a
// partial document. buildCreateWritePlan repeats the same planner after safe
// rewrites because those rewritten bytes are the ones that must be partitioned.
func validateCreateContentPreflight(runtime *common.RuntimeContext) error {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	content := buildCreateContent(runtime)
	_, err := planCreateContentBatches(format, content)
	if err == nil {
		return nil
	}
	return docsCreatePlanningValidationError(format, content, err)
}

func docsCreatePlanningValidationError(format, content string, planningErr error) error {
	var contentErr *docxparse.ContentLimitError
	if errors.As(planningErr, &contentErr) {
		return docsCreateContentLimitValidationError(contentErr)
	}
	var planErr *docxparse.CreateBatchPlanError
	if errors.As(planningErr, &planErr) {
		return docsCreateBatchValidationError(planErr)
	}
	if format == "markdown" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content cannot be safely partitioned with the DocxXML SDK Markdown parser: %v", planningErr).
			WithParam("--content").
			WithHint("fix the malformed Markdown or split it into smaller files before retrying").
			WithCause(planningErr)
	}

	// Strict XML planning can reject legacy shapes that the SDK intentionally
	// repairs. Preserve the old unbatched service-owned parse behavior only when
	// the compatibility DOM fits both content limits and one create request.
	_, compatibleErr := docxparse.ValidateCompatibleXMLCreateLimits(content, createBatchLimits())
	if errors.As(compatibleErr, &contentErr) {
		return docsCreateContentLimitValidationError(contentErr)
	}
	if errors.As(compatibleErr, &planErr) {
		return docsCreateBatchValidationError(planErr)
	}
	return nil
}

func docsCreateContentLimitValidationError(limitErr *docxparse.ContentLimitError) error {
	if limitErr == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "create content analyzer returned an empty limit error")
	}
	switch limitErr.Kind {
	case docxparse.ContentLimitBlockCharacters:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content contains a block with %d UTF-16 code units, exceeding the limit %d", limitErr.Actual, limitErr.Limit).
			WithParam("--content").
			WithHint("split the long text into multiple sibling blocks before retrying")
	case docxparse.ContentLimitTableCells:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content contains a table with %d effective cells, exceeding the limit %d", limitErr.Actual, limitErr.Limit).
			WithParam("--content").
			WithHint("reduce the table size or split it into multiple sibling tables before retrying")
	case docxparse.ContentLimitTableColumns:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content contains a table with %d columns, exceeding the limit %d", limitErr.Actual, limitErr.Limit).
			WithParam("--content").
			WithHint("reduce the number of table columns or split the table before retrying")
	default:
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "unknown create content limit %q", limitErr.Kind)
	}
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
