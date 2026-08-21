// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontract

import "github.com/larksuite/cli/internal/output"

type MessageMentionRequest struct {
	IDs []string
	All bool
}

type MessageMentionConfirmation struct {
	RequestedID string `json:"requested_id"`
	ID          string `json:"id"`
	IDType      string `json:"id_type"`
	Key         string `json:"key"`
}

type MessageMentionResult struct {
	Status                string                       `json:"status"`
	Requested             []string                     `json:"requested"`
	Confirmed             []MessageMentionConfirmation `json:"confirmed"`
	Missing               []string                     `json:"missing"`
	UnattributedRequested []string                     `json:"unattributed_requested,omitempty"`
	All                   string                       `json:"all"`
	RetryScope            string                       `json:"retry_scope"`
}

// BuildMessageMentionResult compares the structured mention request with the
// returned mention entries. Exact open_id matches are confirmed; unmatched or
// ambiguous entries remain unattributed and never authorize replay.
func BuildMessageMentionResult(request MessageMentionRequest, response any) MessageMentionResult {
	requested := append([]string(nil), request.IDs...)
	result := MessageMentionResult{
		Requested:  requested,
		Confirmed:  []MessageMentionConfirmation{},
		Missing:    []string{},
		All:        "not_requested",
		RetryScope: "none",
	}
	if request.All {
		result.All = "accepted_unverified"
		if len(requested) == 0 {
			result.Status = "accepted_unverified"
			return result
		}
	}

	mentions, ambiguous := parseResponseMentions(response)
	confirmed := make([]MessageMentionConfirmation, 0, len(requested))
	confirmedIDs := make(map[string]struct{}, len(requested))
	responseKeys := make(map[string]struct{}, len(mentions))
	unknownEvidence := false
	for _, mention := range mentions {
		if mention.id == "all" || mention.id == "@_all" {
			if !request.All {
				unknownEvidence = true
			}
			continue
		}
		if mention.idType != "open_id" {
			unknownEvidence = true
			continue
		}
		if !contains(requested, mention.id) {
			unknownEvidence = true
			continue
		}
		if _, duplicate := responseKeys[mention.key]; duplicate {
			ambiguous = true
			continue
		}
		responseKeys[mention.key] = struct{}{}
		if _, duplicate := confirmedIDs[mention.id]; duplicate {
			ambiguous = true
			continue
		}
		confirmedIDs[mention.id] = struct{}{}
		confirmed = append(confirmed, MessageMentionConfirmation{
			RequestedID: mention.id,
			ID:          mention.id,
			IDType:      mention.idType,
			Key:         mention.key,
		})
	}

	unresolved := make([]string, 0, len(requested))
	for _, id := range requested {
		if _, ok := confirmedIDs[id]; !ok {
			unresolved = append(unresolved, id)
		}
	}
	if ambiguous || unknownEvidence || len(unresolved) > 0 {
		result.Status = "partial_unattributed"
		result.Confirmed = confirmed
		if len(unresolved) > 0 {
			result.UnattributedRequested = unresolved
		} else {
			// Do not place the same IDs in both confirmed and unattributed
			// sets when extra entries make the result ambiguous.
			result.Confirmed = []MessageMentionConfirmation{}
			result.UnattributedRequested = append([]string(nil), requested...)
		}
		return result
	}
	result.Confirmed = confirmed
	if request.All {
		result.Status = "accepted_unverified"
	} else {
		result.Status = "complete"
	}
	return result
}

type responseMention struct {
	key    string
	id     string
	idType string
}

func parseResponseMentions(response any) ([]responseMention, bool) {
	if response == nil {
		return nil, false
	}
	values, ok := response.([]any)
	if !ok {
		return nil, true
	}
	mentions := make([]responseMention, 0, len(values))
	for _, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			return mentions, true
		}
		mention := responseMention{
			key:    nonEmptyString(object["key"]),
			id:     nonEmptyString(object["id"]),
			idType: nonEmptyString(object["id_type"]),
		}
		if mention.id == "all" || mention.id == "@_all" {
			mentions = append(mentions, mention)
			continue
		}
		if mention.key == "" || mention.id == "" || mention.idType == "" {
			return mentions, true
		}
		mentions = append(mentions, mention)
	}
	return mentions, false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func finalizeMessageMentions(data any) (Result, error) {
	root, err := checkedResponse(data)
	if err != nil {
		return Result{}, err
	}
	raw, present := root["mention_result"]
	if !present {
		return Result{OK: true, Data: root}, nil
	}
	mention, ok := raw.(MessageMentionResult)
	if !ok || !validMentionResultShape(mention) {
		return Result{}, invalidEvidence("mention_result")
	}

	result := Result{OK: true, Data: root}
	switch mention.Status {
	case "complete", "accepted_unverified":
		return result, nil
	case "partial", "partial_unattributed":
		result.OK = false
		result.ExitCode = output.ExitAPI
		return result, nil
	default:
		return Result{}, invalidEvidence("mention_result")
	}
}

func validMentionResultShape(result MessageMentionResult) bool {
	if result.RetryScope != "none" {
		return false
	}
	for _, confirmation := range result.Confirmed {
		if confirmation.RequestedID == "" || confirmation.ID == "" ||
			confirmation.IDType == "" || confirmation.Key == "" {
			return false
		}
	}
	if result.All != "not_requested" && result.All != "accepted_unverified" {
		return false
	}
	switch result.Status {
	case "complete":
		return len(result.Missing) == 0 && result.All == "not_requested"
	case "accepted_unverified":
		return len(result.Missing) == 0 && result.All == "accepted_unverified"
	case "partial":
		return len(result.Requested) > 0 && len(result.Missing) > 0
	case "partial_unattributed":
		return len(result.Requested) > 0 && len(result.Missing) == 0 &&
			len(result.UnattributedRequested) > 0
	default:
		return false
	}
}
