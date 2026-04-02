// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
)

// PaginationOptions contains pagination control options.
type PaginationOptions struct {
	PageLimit int // max pages to fetch; 0 = unlimited (default: 10)
	PageDelay int // ms, default 200
}

// PaginateWithJq aggregates all pages, checks for API errors, then applies a jq filter.
// If checkErr detects an error, the raw result is printed as JSON before returning the error.
func PaginateWithJq(ctx context.Context, ac *APIClient, request RawApiRequest,
	jqExpr string, out io.Writer, pagOpts PaginationOptions,
	checkErr func(interface{}) error) error {
	result, err := ac.PaginateAll(ctx, request, pagOpts)
	if err != nil {
		return output.ErrNetwork("API call failed: %v", err)
	}
	if apiErr := checkErr(result); apiErr != nil {
		output.FormatValue(out, result, output.FormatJSON)
		return apiErr
	}
	return output.JqFilter(out, result, jqExpr)
}

func mergePagedResults(w io.Writer, results []interface{}) interface{} {
	if len(results) == 0 {
		return map[string]interface{}{}
	}

	firstMap, ok := results[0].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"pages": results}
	}

	data, ok := firstMap["data"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"pages": results}
	}

	arrayField := output.FindArrayField(data)
	if arrayField == "" {
		return map[string]interface{}{"pages": results}
	}

	var merged []interface{}
	for _, r := range results {
		if rm, ok := r.(map[string]interface{}); ok {
			if d, ok := rm["data"].(map[string]interface{}); ok {
				if items, ok := d[arrayField].([]interface{}); ok {
					merged = append(merged, items...)
				}
			}
		}
	}

	fmt.Fprintf(w, "[pagination] merged %d pages, %d total items\n", len(results), len(merged))

	mergedData := make(map[string]interface{})
	for k, v := range data {
		mergedData[k] = v
	}
	mergedData[arrayField] = merged
	mergedData["has_more"] = false
	delete(mergedData, "page_token")
	delete(mergedData, "next_page_token")

	result := make(map[string]interface{})
	for k, v := range firstMap {
		result[k] = v
	}
	result["data"] = mergedData

	return result
}
