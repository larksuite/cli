// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseBaseUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+base-update",
	Description: "Update Base settings",
	Risk:        "write",
	Scopes:      []string{"base:app:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "time-zone", Desc: "new IANA time zone, e.g. Asia/Shanghai", Required: true},
	},
	Tips: []string{
		"Use an IANA time zone such as Asia/Shanghai.",
		"This command updates Base-level settings and preserves settings that are not included in the request.",
		`Example: lark-cli base +base-update --base-token <base_token> --time-zone Asia/Shanghai`,
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateBaseUpdate(runtime)
	},
	DryRun: dryRunBaseUpdate,
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		return executeBaseUpdate(runtime)
	},
}

func validateBaseUpdate(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("time-zone")) == "" {
		return baseFlagErrorf("--time-zone must not be blank")
	}
	return nil
}

func buildBaseUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"time_zone": strings.TrimSpace(runtime.Str("time-zone")),
	}
}

func dryRunBaseUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		PATCH("/open-apis/base/v3/bases/:base_token").
		Body(buildBaseUpdateBody(runtime)).
		Set("base_token", runtime.Str("base-token"))
}

func executeBaseUpdate(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", runtime.Str("base-token")), nil, buildBaseUpdateBody(runtime))
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"base": data, "updated": true}, nil)
	return nil
}
