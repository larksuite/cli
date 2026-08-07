// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

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

func projectTemplateCategoriesResponse(data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"categories": data["categories"],
	}
}

func projectTemplateListResponse(data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"templates": data["templates"],
		"has_more":  data["has_more"],
		"offset":    data["offset"],
	}
}
