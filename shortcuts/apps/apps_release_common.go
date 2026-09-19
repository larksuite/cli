// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"io"

	"github.com/larksuite/cli/internal/output"
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

func firstReleaseValue(data map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func firstReleaseString(data map[string]interface{}, keys ...string) (string, bool) {
	value, present := firstReleaseValue(data, keys...)
	if !present {
		return "", false
	}
	text, _ := value.(string)
	return text, true
}

func setReleaseAlias(out, source map[string]interface{}, canonical string, aliases ...string) {
	value, present := firstReleaseValue(source, aliases...)
	delete(out, canonical)
	for _, alias := range aliases {
		delete(out, alias)
	}
	if present {
		out[canonical] = value
	}
}

func projectReleaseDetail(data map[string]interface{}) releaseDetailProjection {
	releaseRoot := data
	outer := data
	if release, ok := data["release"].(map[string]interface{}); ok {
		releaseRoot = release
	}

	out := make(map[string]interface{}, len(releaseRoot))
	for key, value := range releaseRoot {
		out[key] = value
	}

	setReleaseAlias(out, releaseRoot, "release_id", "releaseID", "release_id")
	setReleaseAlias(out, releaseRoot, "created_at", "createdAt", "created_at")
	setReleaseAlias(out, releaseRoot, "updated_at", "updatedAt", "updated_at")
	setReleaseAlias(out, releaseRoot, "online_url", "onlineUrl", "online_url")
	setReleaseAlias(out, releaseRoot, "commit_id", "commitID", "commit_id")

	delete(out, "release")
	delete(out, "errorLogs")
	delete(out, "error_logs")
	delete(out, "currentNodeInfo")
	delete(out, "current_node_info")

	if logs, present := projectReleaseErrorLogs(outer); present {
		out["error_logs"] = logs
	}
	currentNode := projectReleaseCurrentNodeInfo(outer)
	if currentNode != nil {
		out["current_node_info"] = currentNode
	}

	createdAt, _ := firstReleaseValue(out, "created_at")
	updatedAt, _ := firstReleaseValue(out, "updated_at")
	releaseID, _ := firstReleaseString(out, "release_id")
	status, _ := firstReleaseString(out, "status")
	commitID, _ := firstReleaseString(out, "commit_id")
	onlineURL, _ := firstReleaseString(out, "online_url")
	return releaseDetailProjection{
		Data:        out,
		ReleaseID:   releaseID,
		Status:      status,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		CommitID:    commitID,
		OnlineURL:   onlineURL,
		CurrentNode: currentNode,
	}
}

func projectReleaseErrorLogs(data map[string]interface{}) ([]interface{}, bool) {
	raw, present := firstReleaseValue(data, "errorLogs", "error_logs")
	if !present {
		return nil, false
	}
	logs, ok := raw.([]interface{})
	if !ok {
		return []interface{}{}, true
	}

	out := make([]interface{}, 0, len(logs))
	for _, log := range logs {
		entry, ok := log.(map[string]interface{})
		if !ok {
			out = append(out, log)
			continue
		}
		clone := make(map[string]interface{}, len(entry))
		for key, value := range entry {
			clone[key] = value
		}
		setReleaseAlias(clone, entry, "error_log", "errorLog", "error_log")
		out = append(out, clone)
	}
	return out, true
}

func projectReleaseCurrentNodeInfo(data map[string]interface{}) *releaseCurrentNodeInfo {
	raw, present := firstReleaseValue(data, "currentNodeInfo", "current_node_info")
	if !present {
		return nil
	}
	node, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}

	currentNode, _ := firstReleaseString(node, "currentNode", "current_node")
	currentStatus, _ := firstReleaseString(node, "currentStatus", "current_status")
	projection := &releaseCurrentNodeInfo{CurrentNode: currentNode, CurrentStatus: currentStatus}
	if createdAt, ok := firstReleaseValue(node, "createdAt", "created_at"); ok {
		projection.CreatedAt = createdAt
	}

	if rawResult, ok := node["result"].(map[string]interface{}); ok {
		if approvalURL, _ := firstReleaseString(rawResult, "approvalURL", "approval_url"); approvalURL != "" {
			projection.Result = &releaseApprovalResult{ApprovalURL: approvalURL}
		}
	}

	if rawSubmitter, present := firstReleaseValue(node, "submittedBy", "submitted_by"); present {
		if submitter, ok := rawSubmitter.(map[string]interface{}); ok {
			username, _ := firstReleaseString(submitter, "username")
			email, _ := firstReleaseString(submitter, "email")
			openID, _ := firstReleaseString(submitter, "openID", "open_id")
			projected := &releaseSubmittedBy{
				Username: username,
				Email:    email,
				OpenID:   openID,
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
