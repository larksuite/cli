// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const templateReadScope = "base:template:read"

func templatePaginationFlags() []common.Flag {
	return []common.Flag{
		{Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "10", Desc: "pagination size, range 1-100"},
		{Name: "offset", Desc: "pagination cursor from the previous response"},
	}
}

func validateTemplatePagination(_ context.Context, runtime *common.RuntimeContext) error {
	_, err := common.ValidatePageSizeTyped(runtime, "limit", 10, 1, 100)
	return err
}

func templatePaginationParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{
		"limit": runtime.Int("limit"),
	}
	if offset := strings.TrimSpace(runtime.Str("offset")); offset != "" {
		params["offset"] = offset
	}
	return params
}

type templateCategoryItem map[string]interface{}

type templateCategoriesResponse struct {
	Categories []templateCategoryItem `json:"categories"`
}

type templateItem map[string]interface{}

type templateListResponse struct {
	Templates []templateItem `json:"templates"`
	HasMore   bool           `json:"has_more"`
	Offset    string         `json:"offset"`
}

func projectTemplateCategoriesResponse(data map[string]interface{}) (templateCategoriesResponse, error) {
	rawCategories, ok := data["categories"].([]interface{})
	if !ok {
		return templateCategoriesResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "template categories response missing categories array")
	}
	categories := make([]templateCategoryItem, 0, len(rawCategories))
	for index, raw := range rawCategories {
		category, ok := raw.(map[string]interface{})
		if !ok {
			return templateCategoriesResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "template categories response item %d is not an object", index)
		}
		categories = append(categories, templateCategoryItem(category))
	}
	return templateCategoriesResponse{Categories: categories}, nil
}

func projectTemplateListResponse(data map[string]interface{}) (templateListResponse, error) {
	rawTemplates, ok := data["templates"].([]interface{})
	if !ok {
		return templateListResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "template list response missing templates array")
	}
	templates := make([]templateItem, 0, len(rawTemplates))
	for index, raw := range rawTemplates {
		template, ok := raw.(map[string]interface{})
		if !ok {
			return templateListResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "template list response item %d is not an object", index)
		}
		templates = append(templates, templateItem(template))
	}
	hasMore, ok := data["has_more"].(bool)
	if !ok {
		return templateListResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "template list response missing has_more boolean")
	}
	offset, ok := data["offset"].(string)
	if !ok {
		return templateListResponse{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "template list response missing offset string")
	}
	return templateListResponse{Templates: templates, HasMore: hasMore, Offset: offset}, nil
}
