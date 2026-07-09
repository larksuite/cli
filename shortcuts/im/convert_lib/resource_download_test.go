// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// unusedRoundTrip is a transport that fails if the engine ever issues HTTP —
// EnrichResourceDownloads must drive all IO through the injected downloader.
func unusedRoundTrip(t *testing.T) http.RoundTripper {
	t.Helper()
	return convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("EnrichResourceDownloads must not issue HTTP directly, got %s", req.URL.String())
		return nil, nil
	})
}

func resourceRef(messageID, key, fileType string) map[string]interface{} {
	return map[string]interface{}{"message_id": messageID, "key": key, "type": fileType}
}

func TestEnrichResourceDownloads_Dedup(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, unusedRoundTrip(t))

	var mu sync.Mutex
	calls := map[string]int{}
	dl := func(_ context.Context, messageID, key, fileType string) (string, int64, error) {
		mu.Lock()
		calls[messageID+"/"+key]++
		mu.Unlock()
		return "lark-im-resources/" + key, 10, nil
	}

	// Same (message_id, key) appears on two distinct message maps (e.g. mget
	// with a duplicated id). The downloader must run once, both refs fill back.
	messages := []map[string]interface{}{
		{"message_id": "om_1", "resources": []map[string]interface{}{resourceRef("om_1", "k1", "file")}},
		{"message_id": "om_1", "resources": []map[string]interface{}{resourceRef("om_1", "k1", "file")}},
	}

	EnrichResourceDownloads(runtime, messages, dl)

	if calls["om_1/k1"] != 1 {
		t.Fatalf("downloader called %d times for om_1/k1, want 1 (dedup)", calls["om_1/k1"])
	}
	for i, m := range messages {
		refs := m["resources"].([]map[string]interface{})
		if refs[0]["local_path"] != "lark-im-resources/k1" {
			t.Fatalf("message %d ref not filled back: %#v", i, refs[0])
		}
	}
}

// TestEnrichResourceDownloads_BoundedConcurrency deterministically proves the
// semaphore admits exactly resourceDownloadConcurrency downloads at once and
// blocks the next one — without relying on sleep-based peak sampling. Each
// download signals on `entered` then blocks on `release`; we assert that
// exactly `resourceDownloadConcurrency` enter and that one more stays blocked
// until we release.
func TestEnrichResourceDownloads_BoundedConcurrency(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, unusedRoundTrip(t))

	total := resourceDownloadConcurrency + 3
	entered := make(chan struct{}, total)
	release := make(chan struct{})
	dl := func(_ context.Context, messageID, key, fileType string) (string, int64, error) {
		entered <- struct{}{}
		<-release
		return "p/" + key, 1, nil
	}

	messages := make([]map[string]interface{}, total)
	for i := range messages {
		id := fmt.Sprintf("om_%02d", i)
		messages[i] = map[string]interface{}{
			"message_id": id,
			"resources":  []map[string]interface{}{resourceRef(id, fmt.Sprintf("k%02d", i), "file")},
		}
	}

	done := make(chan struct{})
	go func() {
		EnrichResourceDownloads(runtime, messages, dl)
		close(done)
	}()

	// Exactly resourceDownloadConcurrency downloads must start concurrently.
	for i := 0; i < resourceDownloadConcurrency; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d downloads started, want %d concurrent", i, resourceDownloadConcurrency)
		}
	}
	// One more must NOT start while the first batch is still in flight — the
	// semaphore caps it, so the peak can never exceed resourceDownloadConcurrency.
	select {
	case <-entered:
		t.Fatalf("a download beyond the cap (%d) started while the batch was in flight", resourceDownloadConcurrency)
	case <-time.After(200 * time.Millisecond):
		// expected: blocked on the semaphore
	}

	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("EnrichResourceDownloads did not finish after release")
	}
}

func TestEnrichResourceDownloads_FillBack(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, unusedRoundTrip(t))

	dl := func(_ context.Context, messageID, key, fileType string) (string, int64, error) {
		return "lark-im-resources/voice.mp3", 12345, nil
	}

	messages := []map[string]interface{}{
		{"message_id": "om_1", "content": "[Voice]", "resources": []map[string]interface{}{resourceRef("om_1", "a_1", "file")}},
	}

	EnrichResourceDownloads(runtime, messages, dl)

	ref := messages[0]["resources"].([]map[string]interface{})[0]
	if ref["local_path"] != "lark-im-resources/voice.mp3" {
		t.Fatalf("local_path = %#v, want lark-im-resources/voice.mp3", ref["local_path"])
	}
	if ref["size_bytes"] != int64(12345) {
		t.Fatalf("size_bytes = %#v (type %T), want int64(12345)", ref["size_bytes"], ref["size_bytes"])
	}
	if _, ok := ref["error"]; ok {
		t.Fatalf("successful download must not set error: %#v", ref)
	}
}

func TestEnrichResourceDownloads_FailSilent(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, unusedRoundTrip(t))

	dl := func(_ context.Context, messageID, key, fileType string) (string, int64, error) {
		if key == "bad" {
			return "", 0, fmt.Errorf("scope insufficient")
		}
		return "lark-im-resources/" + key, 7, nil
	}

	messages := []map[string]interface{}{
		{"message_id": "om_1", "content": "[File]", "resources": []map[string]interface{}{
			resourceRef("om_1", "bad", "file"),
			resourceRef("om_1", "good", "file"),
		}},
	}

	EnrichResourceDownloads(runtime, messages, dl)

	refs := messages[0]["resources"].([]map[string]interface{})
	if refs[0]["error"] != true {
		t.Fatalf("failed resource must be flagged error:true, got %#v", refs[0])
	}
	if _, ok := refs[0]["local_path"]; ok {
		t.Fatalf("failed resource must not have local_path: %#v", refs[0])
	}
	if refs[1]["local_path"] != "lark-im-resources/good" {
		t.Fatalf("other resource must still download: %#v", refs[1])
	}
	if messages[0]["content"] != "[File]" {
		t.Fatalf("main message content must be untouched, got %#v", messages[0]["content"])
	}
	errOut := runtime.IO().ErrOut.(*bytes.Buffer).String()
	if !strings.Contains(errOut, "warning") {
		t.Fatalf("expected stderr warning for failed download, got %q", errOut)
	}
}

func TestEnrichResourceDownloads_WalksThreadReplies(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, unusedRoundTrip(t))

	var mu sync.Mutex
	seen := map[string]bool{}
	dl := func(_ context.Context, messageID, key, fileType string) (string, int64, error) {
		mu.Lock()
		seen[messageID+"/"+key] = true
		mu.Unlock()
		return "p/" + key, 1, nil
	}

	messages := []map[string]interface{}{
		{
			"message_id": "om_root",
			"resources":  []map[string]interface{}{resourceRef("om_root", "root_key", "image")},
			"thread_replies": []map[string]interface{}{
				{"message_id": "om_reply", "resources": []map[string]interface{}{resourceRef("om_reply", "reply_key", "file")}},
			},
		},
	}

	EnrichResourceDownloads(runtime, messages, dl)

	if !seen["om_root/root_key"] {
		t.Fatalf("root resource not downloaded: %#v", seen)
	}
	if !seen["om_reply/reply_key"] {
		t.Fatalf("thread_reply resource not downloaded (walk missed nested node): %#v", seen)
	}
	reply := messages[0]["thread_replies"].([]map[string]interface{})[0]
	ref := reply["resources"].([]map[string]interface{})[0]
	if ref["local_path"] != "p/reply_key" {
		t.Fatalf("thread_reply ref not filled back: %#v", ref)
	}
}
