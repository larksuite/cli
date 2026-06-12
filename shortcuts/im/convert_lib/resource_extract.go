// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

// ResourceRef is a downloadable resource reference extracted from a message
// during formatting. Type is the download API resource type ("image" or
// "file"); MessageID is the message id used as the download API path parameter.
// For a standalone message that is the message's own id; for a resource carried
// inside a merge_forward it is the TOP-LEVEL container's id, because the
// download API addresses forwarded resources by the container, not the sub-item
// (see extractMergeForwardResourceRefs). The extract stage fills these three
// fields only — local_path and size_bytes are filled later by the download
// enrichment stage.
type ResourceRef struct {
	MessageID string
	Key       string
	Type      string
}

// ExtractResourceRefs returns the downloadable resource refs carried by a
// single message's raw content. It is a pure function (no IO/runtime).
//
// Type mapping (design GAP-002):
//   - image, post img (image_key)            -> type "image"
//   - file, audio, video, media, post media  -> type "file"
//   - sticker                                -> never extracted (unsupported)
//
// For merge_forward the sub-items are not standalone message nodes, so this
// folds them in from mergeSub (the pre-fetched flat sub-item list keyed by the
// merge_forward message_id); each sub-item ref carries the TOP-LEVEL container's
// message_id, since the download API rejects sub-item ids (234003 File not in
// msg) and can only fetch a forwarded resource through the container.
// Refs without a usable key are skipped.
func ExtractResourceRefs(msgType, rawContent, messageID string, mergeSub map[string][]map[string]interface{}) []ResourceRef {
	return extractResourceRefs(msgType, rawContent, messageID, mergeSub, nil)
}

// extractResourceRefs is ExtractResourceRefs with a visited set threaded through
// merge_forward recursion. The set guards against self-referential or cyclic
// prefetch maps — a real merge_forward's flat sub-item list can include the
// container itself (or a nested merge_forward that points back), which would
// otherwise recurse until the stack overflows.
func extractResourceRefs(msgType, rawContent, messageID string, mergeSub map[string][]map[string]interface{}, visited map[string]bool) []ResourceRef {
	switch msgType {
	case "image":
		if key := jsonStringField(rawContent, "image_key"); key != "" {
			return []ResourceRef{{MessageID: messageID, Key: key, Type: "image"}}
		}
	case "file", "audio", "video", "media":
		if key := jsonStringField(rawContent, "file_key"); key != "" {
			return []ResourceRef{{MessageID: messageID, Key: key, Type: "file"}}
		}
	case "post":
		return extractPostResourceRefs(rawContent, messageID)
	case "merge_forward":
		return extractMergeForwardResourceRefs(messageID, mergeSub, visited)
	}
	return nil
}

// extractPostResourceRefs walks a post body's elements and collects img/media
// resource refs.
func extractPostResourceRefs(rawContent, messageID string) []ResourceRef {
	parsed, err := ParseJSONObject(rawContent)
	if err != nil || parsed == nil {
		return nil
	}
	body := unwrapPostLocale(parsed)
	if body == nil {
		return nil
	}
	blocks, _ := body["content"].([]interface{})
	var refs []ResourceRef
	for _, para := range blocks {
		elems, _ := para.([]interface{})
		for _, el := range elems {
			elem, _ := el.(map[string]interface{})
			if elem == nil {
				continue
			}
			switch tag, _ := elem["tag"].(string); tag {
			case "img":
				if key, _ := elem["image_key"].(string); key != "" {
					refs = append(refs, ResourceRef{MessageID: messageID, Key: key, Type: "image"})
				}
			case "media":
				if key, _ := elem["file_key"].(string); key != "" {
					refs = append(refs, ResourceRef{MessageID: messageID, Key: key, Type: "file"})
				}
			}
		}
	}
	return refs
}

// extractMergeForwardResourceRefs folds resources from a merge_forward's
// pre-fetched sub-items. Every collected ref carries the TOP-LEVEL container's
// message_id (messageID here), NOT the sub-item's own id: the resources endpoint
// (GET /open-apis/im/v1/messages/:message_id/resources/:file_key) rejects a
// sub-item id with "234003 File not in msg" and can only fetch a forwarded
// resource through the container that was actually retrieved from the chat.
//
// A sub-item that is itself a merge_forward recurses through the same prefetch
// map (absent keys yield nothing, fail-silent) while keeping the same top-level
// owner — nested merge_forward ids are virtual sub-items too, so they cannot
// own a download either. The visited set breaks cycles: a real API sub-item
// list can contain the container's own id or a back-pointing merge_forward, so
// we expand each merge_forward id at most once.
func extractMergeForwardResourceRefs(messageID string, mergeSub map[string][]map[string]interface{}, visited map[string]bool) []ResourceRef {
	return collectMergeForwardResourceRefs(messageID, messageID, mergeSub, visited)
}

// collectMergeForwardResourceRefs expands the merge_forward identified by
// lookupID and returns its leaf resource refs, each addressed for download by
// ownerID — the top-level merge_forward container the caller fetched. ownerID
// stays fixed across nested merge_forwards while lookupID descends into them.
func collectMergeForwardResourceRefs(ownerID, lookupID string, mergeSub map[string][]map[string]interface{}, visited map[string]bool) []ResourceRef {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[lookupID] {
		return nil
	}
	visited[lookupID] = true

	subItems := mergeSub[lookupID]
	if len(subItems) == 0 {
		return nil
	}
	var refs []ResourceRef
	for _, sub := range subItems {
		subType, _ := sub["msg_type"].(string)
		subID, _ := sub["message_id"].(string)
		subRaw := ""
		if body, ok := sub["body"].(map[string]interface{}); ok {
			subRaw, _ = body["content"].(string)
		}
		if subType == "merge_forward" {
			// Nested merge_forward: descend by its own id, but keep downloading
			// every leaf through the same top-level container (ownerID).
			refs = append(refs, collectMergeForwardResourceRefs(ownerID, subID, mergeSub, visited)...)
			continue
		}
		// Leaf sub-item: its resources download through the container (ownerID),
		// not subID.
		refs = append(refs, extractResourceRefs(subType, subRaw, ownerID, mergeSub, visited)...)
	}
	return refs
}

// jsonStringField parses raw as a JSON object and returns the named string
// field, or "" if parsing fails or the field is missing/non-string.
func jsonStringField(raw, field string) string {
	parsed, err := ParseJSONObject(raw)
	if err != nil {
		return ""
	}
	s, _ := parsed[field].(string)
	return s
}
