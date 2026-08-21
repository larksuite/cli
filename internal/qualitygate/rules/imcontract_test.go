// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"fmt"
	"strings"
	"testing"

	imcatalog "github.com/larksuite/cli/internal/imcontract/catalog"
	qdiff "github.com/larksuite/cli/internal/qualitygate/diff"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestIMLeafCommandsExcludeParentsAndOtherDomains(t *testing.T) {
	index := manifest.Manifest{Commands: []manifest.Command{
		{Path: "im chat", Domain: "im", Runnable: true},
		{Path: "im chat get", Domain: "im", Runnable: true},
		{Path: "im chat list", Domain: "im", Runnable: false},
		{Path: "docs chat get", Domain: "docs", Runnable: true},
	}}
	got := imLeafCommandKeys(index)
	if len(got) != 1 || got[0] != "im chat get" {
		t.Fatalf("IM leaves = %#v, want only runnable child", got)
	}
}

func TestIMContractCoverageReportsMissingAndStaleKeys(t *testing.T) {
	index, contracts := completeIMCoverageFixture()
	contracts = contracts[1:]
	contracts = append(contracts, imcatalog.Contract{
		Key: "im stale command", Strategy: imcatalog.Strategy{Kind: imcatalog.EntityReadKind},
	})
	diags := CheckIMContractCoverage(index, contracts)
	if !hasIMContractDiagnostic(diags, "im resource command00", "no completion contract") {
		t.Fatalf("missing-command diagnostic absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, "im stale command", "does not match") {
		t.Fatalf("stale-key diagnostic absent: %#v", diags)
	}
}

func TestIMContractCoverageReportsMissingIMDomain(t *testing.T) {
	index := manifest.Manifest{Commands: []manifest.Command{
		{Path: "docs +fetch", Domain: "docs", Runnable: true},
	}}
	if leaves := imLeafCommandKeys(index); len(leaves) != 0 {
		t.Fatalf("IM leaves = %#v, want none", leaves)
	}
	diags := CheckIMContractCoverage(index, imcatalog.All())
	if !hasIMContractDiagnostic(diags, "", fmt.Sprintf("IM leaf command count is 0, want %d", expectedIMLeafCommands)) {
		t.Fatalf("missing-domain diagnostic absent: %#v", diags)
	}
}

func TestIMContractCoverageDiagnosticIsNotChangedFileFiltered(t *testing.T) {
	diag := imContractDiagnostic("im +chat-list", "missing")
	got := filterPRDiagnostics(
		".",
		"origin/main",
		qdiff.FromChangedFiles([]string{"skills/lark-doc/SKILL.md"}),
		manifest.Manifest{},
		[]report.Diagnostic{diag},
	)
	if len(got) != 1 || got[0].Rule != imContractCoverageRule {
		t.Fatalf("global IM coverage diagnostic was filtered: %#v", got)
	}
}

func TestIMContractCoverageRejectsRiskAndStrategyShapeMismatches(t *testing.T) {
	index, contracts := completeIMCoverageFixture()
	index.Commands[0].Risk = "write"
	contracts[1] = imcatalog.Contract{
		Key: contracts[1].Key,
		Strategy: imcatalog.Strategy{
			Kind:     imcatalog.RequiredResultKind,
			Required: imcatalog.RequiredSpec{Shape: imcatalog.RequiredNestedString, Field: "message"},
		},
		ReplayMode: imcatalog.ReplayForbidden,
	}
	contracts[2] = imcatalog.Contract{
		Key: contracts[2].Key,
		Strategy: imcatalog.Strategy{
			Kind:            imcatalog.SearchReadKind,
			CollectionField: "",
		},
	}
	diags := CheckIMContractCoverage(index, contracts)
	if !hasIMContractDiagnostic(diags, index.Commands[0].Path, "requires command risk read") {
		t.Fatalf("read/write risk diagnostic absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, index.Commands[1].Path, "requires a child field") {
		t.Fatalf("required shape diagnostic absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, index.Commands[2].Path, "requires collection field") {
		t.Fatalf("search shape diagnostic absent: %#v", diags)
	}
}

func TestIMContractCoverageAllowsMaterializeReadToWriteLocalOutput(t *testing.T) {
	index, contracts := completeIMCoverageFixture()
	index.Commands[0].Risk = "write"
	contracts[0] = imcatalog.Contract{
		Key:      contracts[0].Key,
		Strategy: imcatalog.Strategy{Kind: imcatalog.MaterializeReadKind},
	}
	diags := CheckIMContractCoverage(index, contracts)
	for _, diagnostic := range diags {
		if diagnostic.CommandPath == index.Commands[0].Path &&
			strings.Contains(diagnostic.Message, "risk") {
			t.Fatalf("materialize-read local write risk was rejected: %#v", diagnostic)
		}
	}
}

func TestIMContractCoverageRejectsUnknownKindAndNonIMKey(t *testing.T) {
	index, contracts := completeIMCoverageFixture()
	contracts[0] = imcatalog.Contract{
		Key: "docs resource command00", Strategy: imcatalog.Strategy{Kind: imcatalog.StrategyKind("mystery")},
	}
	diags := CheckIMContractCoverage(index, contracts)
	if !hasIMContractDiagnostic(diags, "docs resource command00", "must start with") ||
		!hasIMContractDiagnostic(diags, "docs resource command00", "unknown strategy kind") {
		t.Fatalf("unknown/non-IM diagnostics absent: %#v", diags)
	}
}

func TestIMContractCoverageRestrictsAcceptanceOnlyContracts(t *testing.T) {
	if len(acceptanceOnlyCommandAllowlist) != 1 {
		t.Fatalf("acceptance-only allowlist = %#v, want only moderation update", acceptanceOnlyCommandAllowlist)
	}
	if _, ok := acceptanceOnlyCommandAllowlist["im chat.moderation update"]; !ok {
		t.Fatalf("acceptance-only allowlist = %#v, want moderation update", acceptanceOnlyCommandAllowlist)
	}

	index, contracts := completeIMCoverageFixture()
	if diags := CheckIMContractCoverage(index, contracts); len(diags) != 0 {
		t.Fatalf("valid acceptance-only contract rejected: %#v", diags)
	}

	allowed := len(contracts) - 1
	contracts[allowed].ReplayMode = imcatalog.ReplaySafe
	if diags := CheckIMContractCoverage(index, contracts); !hasIMContractDiagnostic(
		diags,
		"im chat.moderation update",
		"requires replay mode \"forbidden\"",
	) {
		t.Fatalf("replay-safe acceptance-only contract was not rejected: %#v", diags)
	}

	index, contracts = completeIMCoverageFixture()
	index.Commands[0].Risk = "write"
	contracts[0].Strategy = imcatalog.Strategy{Kind: imcatalog.AcceptanceOnlyKind}
	contracts[0].ReplayMode = imcatalog.ReplayForbidden
	if diags := CheckIMContractCoverage(index, contracts); !hasIMContractDiagnostic(
		diags,
		index.Commands[0].Path,
		"is not allowed for this IM command",
	) {
		t.Fatalf("non-allowlisted acceptance-only contract was not rejected: %#v", diags)
	}

	index, contracts = completeIMCoverageFixture()
	contracts[len(contracts)-1] = imcatalog.Contract{
		Key: "im chat.moderation update",
		Strategy: imcatalog.Strategy{
			Kind:     imcatalog.RequiredResultKind,
			Required: imcatalog.RequiredSpec{Shape: imcatalog.RequiredNestedString, Field: "data"},
		},
		ReplayMode: imcatalog.ReplayForbidden,
	}
	if diags := CheckIMContractCoverage(index, contracts); !hasIMContractDiagnostic(
		diags,
		"im chat.moderation update",
		"allowlist entry is stale",
	) {
		t.Fatalf("stale acceptance-only allowlist was not rejected: %#v", diags)
	}
}

func TestIMContractCoverageRejectsIncompleteAndContradictoryEvidence(t *testing.T) {
	index, contracts := completeIMCoverageFixture()
	index.Commands[0].Risk = "write"
	index.Commands[1].Risk = "write"
	index.Commands[2].Risk = "write"
	contracts[0] = imcatalog.Contract{
		Key: contracts[0].Key,
		Strategy: imcatalog.Strategy{
			Kind:     imcatalog.BatchPartialKind,
			Request:  imcatalog.EvidenceSpec{Shape: imcatalog.EvidenceStrings, Field: "ids"},
			Failures: []imcatalog.EvidenceSpec{{Shape: imcatalog.EvidenceObjects, Field: "failed"}},
		},
	}
	ledger := imcatalog.EvidenceSpec{Shape: imcatalog.EvidenceStatusObjects, Field: "results", IDField: "id"}
	contracts[1] = imcatalog.Contract{
		Key: contracts[1].Key,
		Strategy: imcatalog.Strategy{
			Kind:         imcatalog.BatchPartialKind,
			Request:      imcatalog.EvidenceSpec{Shape: imcatalog.EvidenceStrings, Field: "ids"},
			ResultLedger: &ledger,
		},
	}
	contracts[2] = imcatalog.Contract{
		Key: contracts[2].Key,
		Strategy: imcatalog.Strategy{
			Kind:         imcatalog.ResponseSetAssertionKind,
			Request:      imcatalog.EvidenceSpec{Shape: imcatalog.EvidenceStrings, Field: "ids"},
			ResponseSets: []imcatalog.EvidenceSpec{{Shape: imcatalog.EvidenceNestedObjects, Field: "members", IDField: "id"}},
			Assertion:    imcatalog.AssertRequestedPresent,
		},
	}

	diags := CheckIMContractCoverage(index, contracts)
	if !hasIMContractDiagnostic(diags, index.Commands[0].Path, "failure[0] evidence requires an ID field") {
		t.Fatalf("failure shape diagnostic absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, index.Commands[1].Path, "result ledger cannot be combined") {
		t.Fatalf("contradictory ledger diagnostic absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, index.Commands[2].Path, "response set[0] nested evidence requires container and ID fields") {
		t.Fatalf("response-set shape diagnostic absent: %#v", diags)
	}
}

func TestIMContractCoverageRejectsFieldsFromAnotherStrategyKind(t *testing.T) {
	index, contracts := completeIMCoverageFixture()
	contracts[0].Strategy.Required = imcatalog.RequiredSpec{
		Shape: imcatalog.RequiredTopString,
		Field: "message_id",
	}
	contracts[1].Strategy.ResponseSets = []imcatalog.EvidenceSpec{{
		Shape: imcatalog.EvidenceStrings,
		Field: "items",
	}}
	contracts[2].Strategy.CollectionField = "items"

	diags := CheckIMContractCoverage(index, contracts)
	if !hasIMContractDiagnostic(diags, index.Commands[0].Path, "entity_read must not set strategy field required") {
		t.Fatalf("entity/required contradiction absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, index.Commands[1].Path, "entity_read must not set strategy field response_sets") {
		t.Fatalf("entity/response-set contradiction absent: %#v", diags)
	}
	if !hasIMContractDiagnostic(diags, index.Commands[2].Path, "entity_read must not set strategy field collection_field") {
		t.Fatalf("entity/search contradiction absent: %#v", diags)
	}
}

func completeIMCoverageFixture() (manifest.Manifest, []imcatalog.Contract) {
	index := manifest.Manifest{SchemaVersion: 1}
	contracts := make([]imcatalog.Contract, 0, expectedIMLeafCommands)
	for i := 0; i < expectedIMLeafCommands; i++ {
		key := fmt.Sprintf("im resource command%02d", i)
		risk := "read"
		contract := imcatalog.Contract{
			Key: imcatalog.ContractKey(key), Strategy: imcatalog.Strategy{Kind: imcatalog.EntityReadKind},
		}
		if i == expectedIMLeafCommands-1 {
			key = "im chat.moderation update"
			risk = "write"
			contract = imcatalog.Contract{
				Key:        imcatalog.ContractKey(key),
				Strategy:   imcatalog.Strategy{Kind: imcatalog.AcceptanceOnlyKind},
				ReplayMode: imcatalog.ReplayForbidden,
			}
		}
		index.Commands = append(index.Commands, manifest.Command{Path: key, Domain: "im", Runnable: true, Risk: risk})
		contracts = append(contracts, contract)
	}
	return index, contracts
}

func hasIMContractDiagnostic(diags []report.Diagnostic, key, text string) bool {
	for _, diag := range diags {
		if diag.CommandPath == key && strings.Contains(diag.Message, text) {
			return true
		}
	}
	return false
}
