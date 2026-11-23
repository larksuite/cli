// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// Gateway paths for the spark app.release OpenAPI methods.
// Prefix reuses apiBasePath = "/open-apis/spark/v1" (same package).
// Each path contains %s placeholders; use fmt.Sprintf to build the final URL.
const (
	releaseCreatePath = apiBasePath + "/apps/%s/releases"
	releaseGetPath    = apiBasePath + "/apps/%s/releases/%s"
	releaseListPath   = apiBasePath + "/apps/%s/releases"
)

type releaseApprovalResult struct {
	ApprovalURL string `json:"approval_url,omitempty"`
}

type releaseSubmittedBy struct {
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	OpenID   string `json:"open_id,omitempty"`
}

type releaseCurrentNodeInfo struct {
	CurrentNode   string                 `json:"current_node,omitempty"`
	CurrentStatus string                 `json:"current_status,omitempty"`
	Result        *releaseApprovalResult `json:"result,omitempty"`
	SubmittedBy   *releaseSubmittedBy    `json:"submitted_by,omitempty"`
	CreatedAt     interface{}            `json:"created_at,omitempty"`
}

type releaseDetailProjection struct {
	Data        map[string]interface{}
	ReleaseID   string
	Status      string
	CreatedAt   interface{}
	UpdatedAt   interface{}
	CommitID    string
	OnlineURL   string
	CurrentNode *releaseCurrentNodeInfo
}

func projectReleaseDetail(data map[string]interface{}) releaseDetailProjection {
	releaseRoot := data
	if release, ok := data["release"].(map[string]interface{}); ok {
		releaseRoot = release
	}

	out := make(map[string]interface{}, len(releaseRoot))
	for key, value := range releaseRoot {
		out[key] = value
	}

	delete(out, "release")
	if logs, present := releaseDetailAuxiliaryField(data, releaseRoot, "error_logs"); present {
		out["error_logs"] = logs
	} else {
		delete(out, "error_logs")
	}
	rawCurrentNode, currentNodePresent := releaseDetailAuxiliaryField(data, releaseRoot, "current_node_info")
	if currentNodePresent {
		rawCurrentNode = normalizeReleaseCurrentNodeInfo(rawCurrentNode)
		out["current_node_info"] = rawCurrentNode
	} else {
		delete(out, "current_node_info")
	}
	currentNode := projectReleaseCurrentNodeInfo(rawCurrentNode)

	return releaseDetailProjection{
		Data:        out,
		ReleaseID:   common.GetString(out, "release_id"),
		Status:      common.GetString(out, "status"),
		CreatedAt:   out["created_at"],
		UpdatedAt:   out["updated_at"],
		CommitID:    common.GetString(out, "commit_id"),
		OnlineURL:   common.GetString(out, "online_url"),
		CurrentNode: currentNode,
	}
}

// releaseDetailAuxiliaryField selects an auxiliary release field. Newer
// responses put approval context and failure logs next to the release object,
// while compatible responses may keep them inside release. The outer value
// wins when both are present.
func releaseDetailAuxiliaryField(outer, releaseRoot map[string]interface{}, key string) (interface{}, bool) {
	if value, present := outer[key]; present {
		return value, true
	}
	value, present := releaseRoot[key]
	return value, present
}

// normalizeReleaseCurrentNodeInfo adapts the mixed casing returned by the
// release service into the shortcut's stable snake_case output contract. It
// preserves unknown fields and does not mutate the gateway response.
func normalizeReleaseCurrentNodeInfo(raw interface{}) interface{} {
	node, ok := raw.(map[string]interface{})
	if !ok {
		return raw
	}

	normalized := cloneReleaseMap(node)
	normalizeReleaseAlias(normalized, node, "current_node", "currentNode", releaseNonEmptyString)
	normalizeReleaseAlias(normalized, node, "current_status", "currentStatus", releaseNonEmptyString)
	normalizeReleaseAlias(normalized, node, "created_at", "createdAt", releaseNonNilValue)
	normalizeReleaseAlias(normalized, node, "submitted_by", "submittedBy", releaseMapValue)

	if result, ok := normalized["result"].(map[string]interface{}); ok {
		normalizedResult := cloneReleaseMap(result)
		normalizeReleaseAlias(normalizedResult, result, "approval_url", "approvalURL", releaseNonEmptyString)
		normalized["result"] = normalizedResult
	}
	if submitter, ok := normalized["submitted_by"].(map[string]interface{}); ok {
		normalizedSubmitter := cloneReleaseMap(submitter)
		normalizeReleaseAlias(normalizedSubmitter, submitter, "open_id", "openID", releaseNonEmptyString)
		normalized["submitted_by"] = normalizedSubmitter
	}

	return normalized
}

func cloneReleaseMap(source map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func normalizeReleaseAlias(
	normalized, source map[string]interface{},
	canonicalKey, compatibleKey string,
	isValid func(interface{}) bool,
) {
	canonicalValue, canonicalPresent := source[canonicalKey]
	compatibleValue, compatiblePresent := source[compatibleKey]
	delete(normalized, compatibleKey)

	switch {
	case canonicalPresent && isValid(canonicalValue):
		normalized[canonicalKey] = canonicalValue
	case compatiblePresent && isValid(compatibleValue):
		normalized[canonicalKey] = compatibleValue
	case canonicalPresent:
		normalized[canonicalKey] = canonicalValue
	case compatiblePresent:
		normalized[canonicalKey] = compatibleValue
	}
}

func releaseNonEmptyString(value interface{}) bool {
	text, ok := value.(string)
	return ok && text != ""
}

func releaseNonNilValue(value interface{}) bool {
	if text, ok := value.(string); ok {
		return text != ""
	}
	return value != nil
}

func releaseMapValue(value interface{}) bool {
	_, ok := value.(map[string]interface{})
	return ok
}

func projectReleaseCurrentNodeInfo(raw interface{}) *releaseCurrentNodeInfo {
	node, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}

	projection := &releaseCurrentNodeInfo{
		CurrentNode:   common.GetString(node, "current_node"),
		CurrentStatus: common.GetString(node, "current_status"),
	}
	if createdAt, ok := node["created_at"]; ok {
		projection.CreatedAt = createdAt
	}

	if rawResult, ok := node["result"].(map[string]interface{}); ok {
		if approvalURL := common.GetString(rawResult, "approval_url"); approvalURL != "" {
			projection.Result = &releaseApprovalResult{ApprovalURL: approvalURL}
		}
	}

	if rawSubmitter, present := node["submitted_by"]; present {
		if submitter, ok := rawSubmitter.(map[string]interface{}); ok {
			projected := &releaseSubmittedBy{
				Username: common.GetString(submitter, "username"),
				Email:    common.GetString(submitter, "email"),
				OpenID:   common.GetString(submitter, "open_id"),
			}
			if projected.Username != "" || projected.Email != "" || projected.OpenID != "" {
				projection.SubmittedBy = projected
			}
		}
	}

	return projection
}

// writeReleaseErrorLogTable renders a release's error_logs (a slice of
// {step, error_log} maps from the gateway) as a two-column step/error_log
// table via output.PrintTable. Used by +release-get to render a failed
// release's error_logs. A nil/non-slice or
// empty value yields an empty table (PrintTable prints "(no data)").
func writeReleaseErrorLogTable(w io.Writer, raw interface{}) {
	logs, _ := raw.([]interface{})
	rows := make([]map[string]interface{}, 0, len(logs))
	for _, l := range logs {
		m, ok := l.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"step":      releasePrettyDisplayValue(m["step"]),
			"error_log": releasePrettyDisplayValue(m["error_log"]),
		})
	}
	output.PrintTable(w, rows)
}
