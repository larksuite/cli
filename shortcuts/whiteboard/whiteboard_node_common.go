// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type wbNodeBatchPayload struct {
	Nodes []map[string]interface{} `json:"nodes"`
}

// wbNodeBatchSourcePayload accepts both the canonical {"nodes":[...]} input and
// unnormalized raw/query success payloads shaped as {"data":{"nodes":[...]}}.
// The parser normalizes either form into wbNodeBatchPayload before execution.
type wbNodeBatchSourcePayload struct {
	Nodes []map[string]interface{} `json:"nodes"`
	Data  *wbNodeBatchPayload      `json:"data"`
}

func parseWbNodeBatchPayload(raw []byte, requireID bool) (wbNodeBatchPayload, error) {
	var document json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return wbNodeBatchPayload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unmarshal input json failed: %v", err).
			WithParam("--source").
			WithCause(err)
	}

	var source wbNodeBatchSourcePayload
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	if err := decoder.Decode(&source); err != nil {
		return wbNodeBatchPayload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "unmarshal input json failed: %v", err).
			WithParam("--source").
			WithCause(err)
	}
	payload := wbNodeBatchPayload{Nodes: source.Nodes}
	if len(payload.Nodes) == 0 && source.Data != nil {
		payload = *source.Data
	}
	if len(payload.Nodes) == 0 {
		return wbNodeBatchPayload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, `--source must include non-empty "nodes"`).
			WithParam("--source")
	}
	if requireID {
		for i, node := range payload.Nodes {
			id, ok := node["id"].(string)
			if !ok || strings.TrimSpace(id) == "" {
				return wbNodeBatchPayload{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "nodes[%d].id must be a non-empty string", i).
					WithParam("--source")
			}
		}
	}
	return payload, nil
}

func parseWbNodeIDs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-ids is required").
			WithParam("--node-ids")
	}

	parts := strings.Split(raw, ",")
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for i, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--node-ids item %d must not be empty", i+1).
				WithParam("--node-ids")
		}
		if _, ok := seen[id]; ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate node id %q", id).
				WithParam("--node-ids")
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func validateOptionalWhiteboardNodeIdempotentToken(raw string) error {
	if err := common.RejectDangerousCharsTyped("--idempotent-token", raw); err != nil {
		return err
	}
	if raw != "" && len(raw) < 10 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--idempotent-token must be at least 10 characters long.").
			WithParam("--idempotent-token")
	}
	return nil
}
