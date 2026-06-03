// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseAdvpermEnable = common.Shortcut{
	Service:     "base",
	Command:     "+advperm-enable",
	Description: "Enable advanced permissions for a Base",
	Risk:        "write",
	Scopes:      []string{"base:app:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
	},
	Tips: []string{
		"Caller must be a Base admin; enable advanced permissions before creating or updating roles.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PUT("/open-apis/base/v3/bases/:base_token/advperm/enable?enable=true").
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("enable", "true")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodPut,
			ApiPath:     fmt.Sprintf("/open-apis/base/v3/bases/%s/advperm/enable", validate.EncodePathSegment(baseToken)),
			QueryParams: queryParams,
		})
		if err != nil {
			return err
		}

		return handleRoleResponse(runtime, apiResp.RawBody, "enable advanced permissions failed")
	},
}
