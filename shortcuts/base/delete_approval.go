// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const deleteApprovalValidatePath = "/space/api/base/delete/validate-auth-code"
const deleteApprovalRequestPath = "/space/api/base/delete/requests"

type deleteApprovalSpec struct {
	Action       string
	BaseToken    string
	ResourceType string
	ResourceID   string
}

func deleteApprovalFlags() []common.Flag {
	return []common.Flag{
		{Name: "auth-code", Desc: "one-time delete authorization code returned by the Base delete approval page"},
		{Name: "prepare-approval", Type: "bool", Desc: "create a delete approval request instead of executing the delete"},
	}
}

func appendDeleteApprovalFlags(flags ...common.Flag) []common.Flag {
	return append(flags, deleteApprovalFlags()...)
}

func handleDeleteApproval(runtime *common.RuntimeContext, spec deleteApprovalSpec) (bool, error) {
	if runtime.Bool("prepare-approval") {
		return true, prepareDeleteApproval(runtime, spec)
	}
	if strings.TrimSpace(runtime.Str("auth-code")) == "" {
		return false, baseFlagErrorf("--auth-code is required for %s; run with --prepare-approval first to create an approval URL", spec.Action)
	}
	return false, validateDeleteApproval(runtime, spec)
}

func prepareDeleteApproval(runtime *common.RuntimeContext, spec deleteApprovalSpec) error {
	data, err := runtime.CallAPITyped(http.MethodPost, deleteApprovalRequestPath, nil, map[string]interface{}{
		"action":        spec.Action,
		"base_token":    spec.BaseToken,
		"resource_type": spec.ResourceType,
		"resource_id":   spec.ResourceID,
	})
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{
		"approval_required": true,
		"request_id":        common.GetString(data, "request_id"),
		"approval_url":      common.GetString(data, "approval_url"),
		"expires_at":        data["expires_at"],
		"request_digest":    common.GetString(data, "request_digest"),
		"action":            spec.Action,
		"base_token":        spec.BaseToken,
		"resource_type":     spec.ResourceType,
		"resource_id":       spec.ResourceID,
	}, nil)
	return nil
}

func validateDeleteApproval(runtime *common.RuntimeContext, spec deleteApprovalSpec) error {
	data, err := runtime.CallAPITyped(http.MethodPost, deleteApprovalValidatePath, nil, map[string]interface{}{
		"auth_code":      strings.TrimSpace(runtime.Str("auth-code")),
		"action":         spec.Action,
		"base_token":     spec.BaseToken,
		"resource_type":  spec.ResourceType,
		"resource_id":    spec.ResourceID,
		"request_digest": deleteApprovalDigest(spec),
	})
	if err != nil {
		return err
	}
	if code := common.GetString(data, "error_code"); code != "" {
		msg := common.GetString(data, "error_message")
		if msg == "" {
			msg = code
		}
		return baseValidationErrorf("delete authorization failed: %s", msg)
	}
	return nil
}

func deleteApprovalDigest(spec deleteApprovalSpec) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", spec.Action, spec.BaseToken, spec.ResourceType, spec.ResourceID)))
	return hex.EncodeToString(sum[:])
}
