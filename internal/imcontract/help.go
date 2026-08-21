// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import (
	"strings"

	"github.com/spf13/cobra"
)

const (
	helpContractAnnotation = "imcontract.help.contract-key"
	helpSameKeyReplay      = "Idempotent retry: generate the key outside this command, then reuse the same literal with unchanged parameters on every retry."
)

func AnnotateHelpContract(cmd *cobra.Command, key ContractKey) {
	if cmd == nil || key == "" {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[helpContractAnnotation] = string(key)
}

func HelpText(cmd *cobra.Command) string {
	if cmd == nil || !cmd.Runnable() || cmd.Annotations == nil {
		return ""
	}
	contract, ok := Lookup(ContractKey(cmd.Annotations[helpContractAnnotation]))
	if !ok {
		return ""
	}
	var lines []string
	if policy := contract.HelpPolicy.Text(); policy != "" {
		lines = append(lines, policy)
	}
	if contract.ReplayMode == ReplaySameIdempotencyKey {
		lines = append(lines, helpSameKeyReplay)
	}
	return strings.Join(lines, "\n")
}
