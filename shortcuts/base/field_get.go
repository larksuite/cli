// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

var BaseFieldGet = common.Shortcut{
	Service:     "base",
	Command:     "+field-get",
	Description: "Get a field by ID or name",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(false),
		{Name: "field-id-or-name", Desc: "hidden alias for --field-id", Hidden: true},
	},
	Tips: []string{
		`Example: lark-cli base +field-get --base-token <base_token> --table-id <table_id> --field-id "Status"`,
		"field-id accepts a field ID (fld...) or the field name from the current table.",
		"Returns full field configuration; use it as the baseline before +field-update.",
	},
	PostMount: func(cmd *cobra.Command) {
		cmd.MarkFlagsOneRequired("field-id", "field-id-or-name")
		cmd.MarkFlagsMutuallyExclusive("field-id", "field-id-or-name")
		previousPreRunE := cmd.PreRunE
		cmd.PreRunE = func(c *cobra.Command, args []string) error {
			if previousPreRunE != nil {
				if err := previousPreRunE(c, args); err != nil {
					return err
				}
			}
			fieldIDSet := c.Flags().Changed("field-id")
			aliasSet := c.Flags().Changed("field-id-or-name")
			if !fieldIDSet && !aliasSet {
				return baseFlagErrorf("--field-id is required")
			}
			if fieldIDSet && aliasSet {
				return baseFlagErrorf("--field-id and --field-id-or-name are mutually exclusive; use --field-id")
			}
			return nil
		}
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateFieldGet(runtime)
	},
	DryRun: dryRunFieldGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeFieldGet(runtime)
	},
}

func fieldGetRef(runtime *common.RuntimeContext) string {
	if fieldRef := strings.TrimSpace(runtime.Str("field-id")); fieldRef != "" {
		return fieldRef
	}
	return strings.TrimSpace(runtime.Str("field-id-or-name"))
}

func validateFieldGet(runtime *common.RuntimeContext) error {
	fieldID := strings.TrimSpace(runtime.Str("field-id"))
	alias := strings.TrimSpace(runtime.Str("field-id-or-name"))
	if fieldID == "" && alias == "" {
		return baseFlagErrorf("--field-id is required")
	}
	if fieldID != "" && alias != "" {
		return baseFlagErrorf("--field-id and --field-id-or-name are mutually exclusive; use --field-id")
	}
	return nil
}
