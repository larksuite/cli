// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/shortcuts/common"
)

// callMeetingManagementAPIEnvelope keeps the successful OpenAPI envelope for
// the meeting-management commands while reusing the shared typed response
// classifier. Most shortcuts intentionally expose only the server's data field;
// these commands also expose code, msg, log_id, and future top-level fields.
func callMeetingManagementAPIEnvelope(runtime *common.RuntimeContext, method, path string, body any) (map[string]any, map[string]any, error) {
	req := &larkcore.ApiReq{
		HttpMethod: method,
		ApiPath:    path,
	}
	if body != nil {
		req.Body = body
	}

	resp, err := runtime.DoAPI(req)
	if err != nil {
		if errs.IsTyped(err) {
			return nil, nil, err
		}
		return nil, nil, errs.WrapInternal(err)
	}
	if resp == nil {
		return nil, nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting management API returned a nil response")
	}

	data, err := runtime.ClassifyAPIResponse(resp)
	if err != nil {
		return nil, data, err
	}
	parsed, err := client.ParseJSONResponse(resp)
	if err != nil {
		return nil, nil, client.WrapJSONResponseParseError(err, resp.RawBody)
	}
	envelope, ok := parsed.(map[string]any)
	if !ok {
		return nil, nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "meeting management API returned a non-object JSON response")
	}
	if _, present := envelope["log_id"]; !present {
		if logID := resp.Header.Get("x-tt-logid"); logID != "" {
			envelope["log_id"] = logID
		}
	}
	return envelope, data, nil
}
