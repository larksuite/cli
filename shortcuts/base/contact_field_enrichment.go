// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const contactFieldEnrichmentReference = "lark-base contact-field-enrichment reference"

var contactFieldEnrichmentInfoTypes = map[string]struct{}{
	"department":      {},
	"direct_manager":  {},
	"employee_number": {},
	"job_title":       {},
	"employee_type":   {},
	"work_email":      {},
}

type contactFieldEnrichmentCreateRequest struct {
	Enrichments []contactFieldEnrichmentCreateItem `json:"enrichments"`
}

type contactFieldEnrichmentCreateItem struct {
	SourceField string `json:"source_field"`
	InfoType    string `json:"info_type"`
}

var BaseContactFieldEnrichmentList = common.Shortcut{
	Service:     "base",
	Command:     "+contact-field-enrichment-list",
	Description: "List contact field enrichments in a table",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags:       []common.Flag{baseTokenFlag(true), tableRefFlag(true)},
	Tips: []string{
		"Returns the complete enrichment snapshot; error_message explains any generated field that has stopped updating.",
	},
	DryRun: dryRunContactFieldEnrichmentList,
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		return executeContactFieldEnrichmentList(runtime)
	},
}

var BaseContactFieldEnrichmentCreate = common.Shortcut{
	Service:     "base",
	Command:     "+contact-field-enrichment-create",
	Description: "Create contact field enrichments in a table",
	Risk:        "write",
	Scopes:      []string{"base:field:create"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		{Name: "json", Desc: "enrichment JSON object; agents must read the " + contactFieldEnrichmentReference + " before constructing it", Required: true},
	},
	Tips: []string{
		`Example: lark-cli base +contact-field-enrichment-create --base-token <base_token> --table-id <table_id> --json '{"enrichments":[{"source_field":"People","info_type":"department"}]}'`,
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		_, err := parseContactFieldEnrichmentCreateRequest(runtime)
		return err
	},
	DryRun: dryRunContactFieldEnrichmentCreate,
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		return executeContactFieldEnrichmentCreate(runtime)
	},
}

func contactFieldEnrichmentPath(runtime *common.RuntimeContext) string {
	return baseV3Path(
		"bases", runtime.Str("base-token"),
		"tables", baseTableID(runtime),
		"contact_field_enrichments",
	)
}

func dryRunContactFieldEnrichmentList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/contact_field_enrichments").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func dryRunContactFieldEnrichmentCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, err := parseContactFieldEnrichmentCreateRequest(runtime)
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("dry-run validation failed: %v", err))
	}
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/contact_field_enrichments").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime))
}

func parseContactFieldEnrichmentCreateRequest(runtime *common.RuntimeContext) (contactFieldEnrichmentCreateRequest, error) {
	raw, err := loadJSONInput(newParseCtx(runtime), runtime.Str("json"), "json")
	if err != nil {
		return contactFieldEnrichmentCreateRequest{}, err
	}

	var request contactFieldEnrichmentCreateRequest
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return contactFieldEnrichmentCreateRequest{}, formatJSONError("json", "contact field enrichment object", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return contactFieldEnrichmentCreateRequest{}, baseFlagErrorf("--json must contain exactly one JSON object; %s", jsonInputTip("json"))
	}
	if len(request.Enrichments) == 0 {
		return contactFieldEnrichmentCreateRequest{}, baseFlagErrorf("--json.enrichments must contain at least one item")
	}
	for index, item := range request.Enrichments {
		if strings.TrimSpace(item.SourceField) == "" {
			return contactFieldEnrichmentCreateRequest{}, baseFlagErrorf("--json.enrichments[%d].source_field must not be blank", index)
		}
		if _, ok := contactFieldEnrichmentInfoTypes[item.InfoType]; !ok {
			return contactFieldEnrichmentCreateRequest{}, baseFlagErrorf(
				"--json.enrichments[%d].info_type must be one of department, direct_manager, employee_number, job_title, employee_type, work_email; got %q",
				index,
				item.InfoType,
			)
		}
	}
	return request, nil
}

func executeContactFieldEnrichmentList(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, http.MethodGet, contactFieldEnrichmentPath(runtime), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeContactFieldEnrichmentCreate(runtime *common.RuntimeContext) error {
	body, err := parseContactFieldEnrichmentCreateRequest(runtime)
	if err != nil {
		return err
	}
	data, err := baseV3Call(runtime, http.MethodPost, contactFieldEnrichmentPath(runtime), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}
