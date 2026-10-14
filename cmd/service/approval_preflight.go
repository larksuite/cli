// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
)

var unsupportedApprovalInstanceControlTypes = map[string]struct{}{
	"text":                        {},
	"mutableGroup":                {},
	"account":                     {},
	"serialNumber":                {},
	"tripGroup":                   {},
	"apaascorehrOnboardingGroup":  {},
	"apaascorehrRegularateGroup":  {},
	"remedyGroupV2":               {},
	"apaascorehrJobAdjustGroup":   {},
	"apaascorehrOffboardingGroup": {},
}

type unsupportedApprovalControl struct {
	ID   string
	Type string
}

func validateApprovalInstanceCreateData(schemaPath string, data any) error {
	if schemaPath != "approval.instances.create" {
		return nil
	}
	body, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	rawForm, ok := body["form"].(string)
	if !ok || strings.TrimSpace(rawForm) == "" {
		return nil
	}

	var controls []any
	if err := json.Unmarshal([]byte(rawForm), &controls); err != nil {
		return nil
	}
	var unsupported []unsupportedApprovalControl
	collectUnsupportedApprovalControls(controls, &unsupported)
	if len(unsupported) == 0 {
		return nil
	}

	sort.Slice(unsupported, func(i, j int) bool {
		if unsupported[i].Type == unsupported[j].Type {
			return unsupported[i].ID < unsupported[j].ID
		}
		return unsupported[i].Type < unsupported[j].Type
	})
	parts := make([]string, 0, len(unsupported))
	for _, item := range unsupported {
		if item.ID == "" {
			parts = append(parts, item.Type)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%s)", item.Type, item.ID))
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"approval instances create does not support form control type(s): %s",
		strings.Join(parts, ", ")).
		WithParam("--data").
		WithHint("remove unsupported controls from --data.form or create the approval in the Lark/Feishu client")
}

func collectUnsupportedApprovalControls(value any, out *[]unsupportedApprovalControl) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectUnsupportedApprovalControls(item, out)
		}
	case map[string]any:
		controlType, _ := typed["type"].(string)
		if _, unsupported := unsupportedApprovalInstanceControlTypes[controlType]; unsupported {
			id, _ := typed["id"].(string)
			*out = append(*out, unsupportedApprovalControl{ID: id, Type: controlType})
		}
		for _, nested := range typed {
			collectUnsupportedApprovalControls(nested, out)
		}
	}
}
