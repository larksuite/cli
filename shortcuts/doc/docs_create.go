// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

const docsContentPathAnnotation = "lark-cli.docs.content-input-path"

// v1CreateFlags returns hidden parse-only compatibility flags for old v1 commands.
func v1CreateFlags() []common.Flag {
	return docsLegacyFlagDefinitions(docsCreateLegacyFlags())
}

var docsCreateLocalResourceScopes = []string{
	"docs:document.media:upload",
	"docx:document:write_only",
	"docx:document:readonly",
}

var DocsCreate = common.Shortcut{
	Service:           "docs",
	Command:           "+create",
	Description:       "Create a Lark document",
	Risk:              "write",
	AuthTypes:         []string{"user", "bot"},
	Scopes:            []string{"docx:document:create"},
	ConditionalScopes: docsCreateLocalResourceScopes,
	PostMount:         installDocsContentPathCapture,
	Flags: concatFlags(
		[]common.Flag{
			docsAPIVersionCompatFlag(),
			docsOutputFormatCompatFlag(),
			docsJSONOutputCompatFlag(),
		},
		v2CreateFlags(),
		v1CreateFlags(),
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateCreateV2(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunCreateV2(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeCreateV2(ctx, runtime)
	},
}

func installDocsContentPathCapture(cmd *cobra.Command) {
	previousPreRunE := cmd.PreRunE
	cmd.PreRunE = func(command *cobra.Command, args []string) error {
		if previousPreRunE != nil {
			if err := previousPreRunE(command, args); err != nil {
				return err
			}
		}
		captureDocsContentPath(command)
		return nil
	}
}

func captureDocsContentPath(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	delete(cmd.Annotations, docsContentPathAnnotation)
	raw, err := cmd.Flags().GetString("content")
	if err != nil || !strings.HasPrefix(raw, "@") || strings.HasPrefix(raw, "@@") {
		return
	}
	if path := strings.TrimSpace(strings.TrimPrefix(raw, "@")); path != "" {
		cmd.Annotations[docsContentPathAnnotation] = path
	}
}

// concatFlags combines multiple flag slices into one.
func concatFlags(slices ...[]common.Flag) []common.Flag {
	var out []common.Flag
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}
