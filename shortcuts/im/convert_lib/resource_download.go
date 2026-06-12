// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"context"
	"sync"

	"github.com/larksuite/cli/shortcuts/common"
)

// resourceDownloadConcurrency caps in-flight resource downloads. Each download
// is a GET plus a local disk write; capping at 3 keeps the
// messages/{id}/resources/{key} endpoint well under any gateway-layer rate
// ceiling while still cutting wall-clock versus a serial loop.
const resourceDownloadConcurrency = 3

// ResourceDownloader downloads one resource and returns its local path and
// size in bytes. messageID is the resource's owning message id (the download
// API path parameter), key is the file_key/image_key, and fileType is the
// download API resource type ("image" or "file"). A non-nil error means the
// single resource failed; the engine isolates that failure (fail-silent).
type ResourceDownloader func(ctx context.Context, messageID, key, fileType string) (string, int64, error)

// EnrichResourceDownloads walks every message node (including nested
// thread_replies) for "resources" blocks attached during formatting, downloads
// each distinct (message_id, key) once with bounded concurrency, and fills
// local_path/size_bytes back into every ref sharing that key. A single
// resource failing is isolated: its ref is flagged "error": true and a warning
// is written to stderr, while the main message and the other resources are
// unaffected (S2.STA-DES-P0-002 weak-dependency isolation).
func EnrichResourceDownloads(runtime *common.RuntimeContext, messages []map[string]interface{}, dl ResourceDownloader) {
	if len(messages) == 0 || dl == nil {
		return
	}

	type refKey struct {
		messageID string
		key       string
	}
	groups := make(map[refKey][]map[string]interface{})
	types := make(map[refKey]string)
	var order []refKey

	collectResourceRefs(messages, func(ref map[string]interface{}) {
		messageID, _ := ref["message_id"].(string)
		key, _ := ref["key"].(string)
		if messageID == "" || key == "" {
			return
		}
		rk := refKey{messageID: messageID, key: key}
		if _, seen := groups[rk]; !seen {
			order = append(order, rk)
			if t, _ := ref["type"].(string); t != "" {
				types[rk] = t
			}
		}
		groups[rk] = append(groups[rk], ref)
	})
	if len(order) == 0 {
		return
	}

	ctx := runtime.Ctx()
	var stderrMu sync.Mutex

	download := func(rk refKey) {
		if err := ctx.Err(); err != nil {
			return
		}
		localPath, size, err := dl(ctx, rk.messageID, rk.key, types[rk])
		if err != nil {
			warnSyncf(&stderrMu, runtime.IO().ErrOut,
				"warning: resource_download_failed: %s/%s: %v\n", rk.messageID, rk.key, err)
			for _, ref := range groups[rk] {
				ref["error"] = true
			}
			return
		}
		for _, ref := range groups[rk] {
			ref["local_path"] = localPath
			ref["size_bytes"] = size
		}
	}

	// Single-resource fast path: no goroutine overhead, deterministic stderr.
	if len(order) == 1 {
		download(order[0])
		return
	}

	// Bounded-concurrency fan-out. Each goroutine writes only to its own
	// (message_id, key) group's ref maps — distinct keys map to distinct ref
	// maps, so there is no shared mutable state besides the stderr mutex.
	sem := make(chan struct{}, resourceDownloadConcurrency)
	var wg sync.WaitGroup
	for _, rk := range order {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			download(rk)
		}()
	}
	wg.Wait()
}

// collectResourceRefs walks messages (and nested thread_replies) and invokes fn
// for every resource ref map found in each node's "resources" block. Handles
// both the typed []map[string]interface{} (in-memory, set by formatMessageItem)
// and []interface{} (post JSON round-trip) shapes, mirroring collectMessageNodes.
func collectResourceRefs(messages []map[string]interface{}, fn func(ref map[string]interface{})) {
	for _, msg := range messages {
		switch res := msg["resources"].(type) {
		case []map[string]interface{}:
			for _, ref := range res {
				fn(ref)
			}
		case []interface{}:
			for _, raw := range res {
				if ref, ok := raw.(map[string]interface{}); ok {
					fn(ref)
				}
			}
		}
		switch nested := msg["thread_replies"].(type) {
		case []map[string]interface{}:
			collectResourceRefs(nested, fn)
		case []interface{}:
			typed := make([]map[string]interface{}, 0, len(nested))
			for _, raw := range nested {
				if m, ok := raw.(map[string]interface{}); ok {
					typed = append(typed, m)
				}
			}
			collectResourceRefs(typed, fn)
		}
	}
}
