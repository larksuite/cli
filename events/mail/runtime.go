// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"net/url"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

type Runtime struct {
	runtime *common.RuntimeContext
}

func NewRuntime(runtime *common.RuntimeContext) *Runtime {
	return &Runtime{runtime: runtime}
}

func (r *Runtime) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	apiPath := path
	query := make(larkcore.QueryParams)
	if u, err := url.Parse(path); err == nil && u.RawQuery != "" {
		apiPath = u.Path
		for key, values := range u.Query() {
			if len(values) > 0 {
				query.Set(key, values[0])
			}
		}
	}
	data, err := r.runtime.DoAPIJSONTyped(method, apiPath, query, body)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"data": data})
}
