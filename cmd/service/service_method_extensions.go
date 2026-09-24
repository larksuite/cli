// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"context"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

type serviceMethodExtension struct {
	dryRun         func(*cmdutil.Factory, client.RawApiRequest, *core.CliConfig, string) error
	prepareRequest func(context.Context, *client.APIClient, client.RawApiRequest) (client.RawApiRequest, error)
}

var serviceMethodExtensions = map[string]serviceMethodExtension{
	mailRuleReorderSchemaPath: {
		dryRun:         serviceDryRunMailRuleReorder,
		prepareRequest: completeMailRuleReorderRequest,
	},
}

func serviceMethodExtensionFor(schemaPath string) serviceMethodExtension {
	return serviceMethodExtensions[schemaPath]
}
