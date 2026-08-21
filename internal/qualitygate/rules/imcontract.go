// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"fmt"
	"sort"
	"strings"

	imcatalog "github.com/larksuite/cli/internal/imcontract/catalog"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

const (
	imContractCoverageRule = "im_contract_coverage"
	expectedIMLeafCommands = 62
)

var acceptanceOnlyCommandAllowlist = map[imcatalog.ContractKey]struct{}{
	"im chat.moderation update": {},
}

func CheckIMContractCoverage(commandIndex manifest.Manifest, contracts []imcatalog.Contract) []report.Diagnostic {
	leafKeys := imLeafCommandKeys(commandIndex)
	leafSet := make(map[string]struct{}, len(leafKeys))
	for _, key := range leafKeys {
		leafSet[key] = struct{}{}
	}
	contractSet := make(map[string]imcatalog.Contract, len(contracts))
	for _, contract := range contracts {
		contractSet[string(contract.Key)] = contract
	}
	commandByPath := make(map[string]manifest.Command, len(commandIndex.Commands))
	for _, command := range commandIndex.Commands {
		commandByPath[command.Path] = command
	}

	var diags []report.Diagnostic
	for allowedKey := range acceptanceOnlyCommandAllowlist {
		key := string(allowedKey)
		if _, ok := leafSet[key]; !ok {
			diags = append(diags, imContractDiagnostic(
				key,
				"acceptance_only allowlist key does not match a runnable IM leaf command",
			))
		}
		contract, ok := contractSet[key]
		if !ok {
			diags = append(diags, imContractDiagnostic(
				key,
				"acceptance_only allowlist key has no completion contract",
			))
		} else if contract.Strategy.Kind != imcatalog.AcceptanceOnlyKind {
			diags = append(diags, imContractDiagnostic(
				key,
				fmt.Sprintf("acceptance_only allowlist entry is stale for strategy kind %q", contract.Strategy.Kind),
			))
		}
	}
	if len(leafKeys) != expectedIMLeafCommands {
		diags = append(diags, imContractDiagnostic(
			"",
			fmt.Sprintf("IM leaf command count is %d, want %d", len(leafKeys), expectedIMLeafCommands),
		))
	}
	for _, key := range leafKeys {
		if _, ok := contractSet[key]; !ok {
			diags = append(diags, imContractDiagnostic(key, "IM leaf command has no completion contract"))
		}
	}
	for _, contract := range contracts {
		key := string(contract.Key)
		if _, ok := leafSet[key]; !ok {
			diags = append(diags, imContractDiagnostic(key, "IM contract key does not match a runnable leaf command"))
		}
		for _, message := range validateIMContractShape(contract, commandByPath[key]) {
			diags = append(diags, imContractDiagnostic(key, message))
		}
	}
	return diags
}

func validateIMContractShape(contract imcatalog.Contract, command manifest.Command) []string {
	var messages []string
	key := string(contract.Key)
	if !strings.HasPrefix(key, "im ") {
		messages = append(messages, "IM contract key must start with \"im \"")
	}
	kind := contract.Strategy.Kind
	if !kind.IsRead() && !kind.IsWrite() {
		return append(messages, fmt.Sprintf("IM contract has unknown strategy kind %q", kind))
	}
	if command.Path != "" {
		switch {
		case kind == imcatalog.MaterializeReadKind &&
			command.Risk != "read" && command.Risk != "write":
			messages = append(messages, fmt.Sprintf("IM materialize read contract requires command risk read or write, got %q", command.Risk))
		case kind.IsRead() && kind != imcatalog.MaterializeReadKind && command.Risk != "read":
			messages = append(messages, fmt.Sprintf("IM read contract requires command risk read, got %q", command.Risk))
		case kind.IsWrite() && command.Risk != "write" && command.Risk != "high-risk-write":
			messages = append(messages, fmt.Sprintf("IM write contract requires command risk write or high-risk-write, got %q", command.Risk))
		}
	}

	switch kind {
	case imcatalog.AcceptanceOnlyKind:
		if contract.ReplayMode != imcatalog.ReplayForbidden {
			messages = append(messages, fmt.Sprintf(
				"acceptance_only requires replay mode %q, got %q",
				imcatalog.ReplayForbidden,
				contract.ReplayMode,
			))
		}
		if _, ok := acceptanceOnlyCommandAllowlist[contract.Key]; !ok {
			messages = append(messages, "acceptance_only is not allowed for this IM command")
		}
	case imcatalog.RequiredResultKind:
		if message := validateRequiredSpec(contract.Strategy.Required); message != "" {
			messages = append(messages, message)
		}
	case imcatalog.BatchPartialKind:
		if contract.Strategy.ResultLedger == nil {
			if message := validateEvidenceSpec("request", contract.Strategy.Request); message != "" {
				messages = append(messages, message)
			}
			if len(contract.Strategy.Failures) == 0 && len(contract.Strategy.Pending) == 0 {
				messages = append(messages, "batch_partial requires failures, pending evidence, or a result ledger")
			}
			messages = append(messages, validateEvidenceSpecs("failure", contract.Strategy.Failures)...)
			messages = append(messages, validateEvidenceSpecs("pending", contract.Strategy.Pending)...)
		} else {
			if message := validateEvidenceSpec("result ledger", *contract.Strategy.ResultLedger); message != "" {
				messages = append(messages, message)
			}
			if evidenceSpecPresent(contract.Strategy.Request) ||
				len(contract.Strategy.Failures) > 0 || len(contract.Strategy.Pending) > 0 {
				messages = append(messages, "batch_partial result ledger cannot be combined with request, failure, or pending evidence")
			}
		}
	case imcatalog.RequiredResultBatchPartialKind:
		if message := validateRequiredSpec(contract.Strategy.Required); message != "" {
			messages = append(messages, message)
		}
		if message := validateEvidenceSpec("request", contract.Strategy.Request); message != "" {
			messages = append(messages, message)
		}
		if len(contract.Strategy.Failures) == 0 {
			messages = append(messages, "required_result_batch_partial requires failure evidence")
		}
		messages = append(messages, validateEvidenceSpecs("failure", contract.Strategy.Failures)...)
		messages = append(messages, validateEvidenceSpecs("pending", contract.Strategy.Pending)...)
	case imcatalog.ResponseSetAssertionKind:
		if message := validateEvidenceSpec("request", contract.Strategy.Request); message != "" {
			messages = append(messages, message)
		}
		if len(contract.Strategy.ResponseSets) == 0 {
			messages = append(messages, "response_set_assertion requires response sets")
		}
		messages = append(messages, validateEvidenceSpecs("response set", contract.Strategy.ResponseSets)...)
		if contract.Strategy.Assertion != imcatalog.AssertRequestedPresent &&
			contract.Strategy.Assertion != imcatalog.AssertRequestedAbsent {
			messages = append(messages, fmt.Sprintf("response_set_assertion has unknown assertion %q", contract.Strategy.Assertion))
		}
	case imcatalog.SearchReadKind:
		if strings.TrimSpace(contract.Strategy.CollectionField) == "" {
			messages = append(messages, "search_read requires collection field")
		}
	}
	messages = append(messages, validateUnexpectedStrategyFields(contract.Strategy)...)
	return messages
}

func validateUnexpectedStrategyFields(strategy imcatalog.Strategy) []string {
	allowed := map[string]bool{"kind": true}
	switch strategy.Kind {
	case imcatalog.EntityReadKind:
		allowed["read_hint"] = true
	case imcatalog.SearchReadKind:
		allowed["collection_field"] = true
		allowed["requires_materialization"] = true
	case imcatalog.RequiredResultKind:
		allowed["required"] = true
	case imcatalog.BatchPartialKind:
		allowed["request"] = true
		allowed["failures"] = true
		allowed["pending"] = true
		allowed["result_ledger"] = true
	case imcatalog.RequiredResultBatchPartialKind:
		allowed["required"] = true
		allowed["request"] = true
		allowed["failures"] = true
		allowed["pending"] = true
	case imcatalog.ResponseSetAssertionKind:
		allowed["request"] = true
		allowed["response_sets"] = true
		allowed["assertion"] = true
	}

	present := map[string]bool{
		"required":                 requiredSpecPresent(strategy.Required),
		"request":                  evidenceSpecPresent(strategy.Request),
		"failures":                 len(strategy.Failures) > 0,
		"pending":                  len(strategy.Pending) > 0,
		"response_sets":            len(strategy.ResponseSets) > 0,
		"assertion":                strategy.Assertion != "",
		"result_ledger":            strategy.ResultLedger != nil,
		"collection_field":         strategy.CollectionField != "",
		"requires_materialization": strategy.RequiresMaterialization,
		"read_hint":                strategy.ReadHint != "",
	}
	var messages []string
	for field, isPresent := range present {
		if isPresent && !allowed[field] {
			messages = append(messages, fmt.Sprintf("%s must not set strategy field %s", strategy.Kind, field))
		}
	}
	sort.Strings(messages)
	return messages
}

func requiredSpecPresent(spec imcatalog.RequiredSpec) bool {
	return spec.Shape != 0 || spec.Field != "" || spec.Child != ""
}

func validateEvidenceSpecs(label string, specs []imcatalog.EvidenceSpec) []string {
	var messages []string
	for index, spec := range specs {
		indexedLabel := fmt.Sprintf("%s[%d]", label, index)
		if message := validateEvidenceSpec(indexedLabel, spec); message != "" {
			messages = append(messages, message)
		}
	}
	return messages
}

func evidenceSpecPresent(spec imcatalog.EvidenceSpec) bool {
	return spec.Shape != 0 || spec.Field != "" || spec.IDField != "" || spec.Container != ""
}

func validateRequiredSpec(spec imcatalog.RequiredSpec) string {
	if strings.TrimSpace(spec.Field) == "" {
		return "required_result requires a non-empty field"
	}
	switch spec.Shape {
	case imcatalog.RequiredTopString, imcatalog.RequiredTopObject:
		if spec.Child != "" {
			return "top-level required_result must not set child"
		}
	case imcatalog.RequiredNestedString:
		if strings.TrimSpace(spec.Child) == "" {
			return "nested required_result requires a child field"
		}
	default:
		return fmt.Sprintf("required_result has unknown shape %d", spec.Shape)
	}
	return ""
}

func validateEvidenceSpec(label string, spec imcatalog.EvidenceSpec) string {
	if strings.TrimSpace(spec.Field) == "" {
		return label + " evidence requires a non-empty field"
	}
	switch spec.Shape {
	case imcatalog.EvidenceStrings, imcatalog.EvidenceFeedObjects:
	case imcatalog.EvidenceObjects, imcatalog.EvidenceStatusObjects:
		if strings.TrimSpace(spec.IDField) == "" {
			return label + " evidence requires an ID field"
		}
	case imcatalog.EvidenceNestedObjects:
		if strings.TrimSpace(spec.IDField) == "" || strings.TrimSpace(spec.Container) == "" {
			return label + " nested evidence requires container and ID fields"
		}
	case imcatalog.EvidenceNestedFeedObjects:
		if strings.TrimSpace(spec.Container) == "" {
			return label + " nested feed evidence requires a container field"
		}
	default:
		return fmt.Sprintf("%s evidence has unknown shape %d", label, spec.Shape)
	}
	return ""
}

func imLeafCommandKeys(commandIndex manifest.Manifest) []string {
	var candidates []string
	for _, cmd := range commandIndex.Commands {
		if cmd.Domain == "im" && cmd.Runnable {
			candidates = append(candidates, cmd.Path)
		}
	}
	sort.Strings(candidates)
	leaves := make([]string, 0, len(candidates))
	for _, path := range candidates {
		parent := false
		for _, other := range candidates {
			if other != path && strings.HasPrefix(other, path+" ") {
				parent = true
				break
			}
		}
		if !parent {
			leaves = append(leaves, path)
		}
	}
	return leaves
}

func imContractDiagnostic(commandPath, message string) report.Diagnostic {
	return report.Diagnostic{
		Rule:        imContractCoverageRule,
		Action:      report.ActionReject,
		File:        "command-index",
		Message:     message,
		SubjectType: "command",
		CommandPath: commandPath,
	}
}
