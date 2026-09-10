// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

const (
	mailThreadListMethodID        = "user_mailbox.thread.list"
	methodParamContractAnnotation = "method-param-contract"
	mailThreadListSelectorHelp    = "Exactly one of --folder-id or --label-id must be provided; the two options are mutually exclusive."
)

// applyMethodParamContractHelp projects method-specific cross-field contracts
// onto both the command description and each participating optional flag.
func applyMethodParamContractHelp(cmd *cobra.Command, method meta.Method) {
	if method.ID != mailThreadListMethodID {
		return
	}

	cmd.Long += "\n\nParameter constraints:\n  " + mailThreadListSelectorHelp
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[methodParamContractAnnotation] = mailThreadListSelectorHelp

	flagHelp := "Exactly one of --folder-id or --label-id is required; the two options are mutually exclusive."
	for _, name := range []string{"folder-id", "label-id"} {
		if flag := cmd.Flags().Lookup(name); flag != nil {
			if flag.Usage != "" && !strings.HasSuffix(flag.Usage, ".") {
				flag.Usage += "."
			}
			if flag.Usage != "" {
				flag.Usage += " "
			}
			flag.Usage += flagHelp
		}
	}
}

// validateMethodParamContracts enforces cross-field constraints after all
// typed flags have been overlaid onto --params, without changing individual
// fields' required metadata or normalizing their values.
func validateMethodParamContracts(method meta.Method, params map[string]interface{}) error {
	if method.ID != mailThreadListMethodID {
		return nil
	}

	folderValue, folderExists := params["folder_id"]
	labelValue, labelExists := params["label_id"]
	hasFolder := folderExists && !unusableParamValue(folderValue)
	hasLabel := labelExists && !unusableParamValue(labelValue)

	switch {
	case !hasFolder && !hasLabel:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"exactly one of --folder-id or --label-id must be provided").WithParams(
			errs.InvalidParam{Name: "--folder-id", Reason: "exactly one selector is required"},
			errs.InvalidParam{Name: "--label-id", Reason: "exactly one selector is required"},
		)
	case hasFolder && hasLabel:
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--folder-id and --label-id are mutually exclusive; provide exactly one").WithParams(
			errs.InvalidParam{Name: "--folder-id", Reason: "mutually exclusive with --label-id"},
			errs.InvalidParam{Name: "--label-id", Reason: "mutually exclusive with --folder-id"},
		)
	default:
		return nil
	}
}
