// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// fetchFolderChildrenTree unit tests (httpmock, no real openapi).
// Covers XML one-level output (folder key+name+child_count / file key+name /
// sub-folder key+name+child_count / has_more) and fallback paths.

// folderChildrenRoundTrip answers any GET /files/:key/folder request with a
// one-child expansion (or a failure when fail is true). Built on the roundtrip
// runtime with a static token resolver so the concurrent prefetch fan-out does
// not race the credential provider under -race (same approach as merge tests).
func folderChildrenRoundTrip(fail bool) convertlibRoundTripFunc {
	return func(req *http.Request) (*http.Response, error) {
		if fail {
			return convertlibJSONResponse(503, map[string]interface{}{}), nil
		}
		return convertlibJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":     []interface{}{map[string]interface{}{"file_key": "f1", "name": "a.pdf", "is_folder": false}},
				"all_count": float64(1),
			},
		}), nil
	}
}

func folderTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+x"}, cfg, f, core.AsUser)
	return rt, reg
}

const folderURL = "/open-apis/im/v1/files/fld_root/folder?recursive=false&srcid=om_123&srctype=message"

// C4: expand one level (files + sub-folder with child_count), no has_more (items == all_count).
func TestFetchFolderChildrenTree_XMLOneLevel(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"file_key": "f1", "name": "报告.pdf", "is_folder": false},
					map[string]interface{}{"file_key": "f2", "name": "文档.docx", "is_folder": false},
					map[string]interface{}{"file_key": "f3", "name": "子文件夹", "is_folder": true, "children_count": float64(3)},
				},
				"all_count": float64(3),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "tmpavatra", "om_123")
	want := `<folder key="fld_root" name="tmpavatra" child_count="3"><file key="f1" name="报告.pdf"/><file key="f2" name="文档.docx"/><folder key="f3" name="子文件夹" child_count="3"/></folder>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() = %q, want %q", got, want)
	}
}

// C4b: sub-folder with children_count missing (unknown) omits child_count instead of "0".
func TestFetchFolderChildrenTree_SubFolderUnknownCount(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"file_key": "f3", "name": "子文件夹", "is_folder": true}, // no children_count
				},
				"all_count": float64(1),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	want := `<folder key="fld_root" name="x" child_count="1"><folder key="f3" name="子文件夹"/></folder>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() = %q, want %q", got, want)
	}
}

// C4c: items < all_count -> root folder carries has_more="true".
func TestFetchFolderChildrenTree_HasMore(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"file_key": "f1", "name": "a.pdf", "is_folder": false},
				},
				"all_count": float64(100),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "big", "om_123")
	want := `<folder key="fld_root" name="big" child_count="100" has_more="true"><file key="f1" name="a.pdf"/></folder>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() = %q, want %q", got, want)
	}
}

// C4d: more than 10 first-level items -> render first 10 + has_more="true" (rendering cap).
func TestFetchFolderChildrenTree_CapAtTen(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	items := make([]interface{}, 0, 12)
	for i := 0; i < 12; i++ {
		items = append(items, map[string]interface{}{"file_key": fmt.Sprintf("f%d", i), "name": fmt.Sprintf("f%d.pdf", i), "is_folder": false})
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":     items,
				"all_count": float64(12),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "big", "om_123")
	// first 10 rendered, has_more present, last item f11 absent
	if !strings.Contains(got, `child_count="12" has_more="true"`) {
		t.Fatalf("want child_count=12 + has_more, got %q", got)
	}
	if !strings.Contains(got, `<file key="f9" name="f9.pdf"/>`) || strings.Contains(got, `name="f10.pdf"`) {
		t.Fatalf("want first 10 items only, got %q", got)
	}
}

// C5: transport error (no stub registered) -> "" so callers fall back; warning goes to stderr.
func TestFetchFolderChildrenTree_APIFailure(t *testing.T) {
	rt, _ := folderTestRuntime(t)
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	if got != "" {
		t.Fatalf("fetchFolderChildrenTree() on API failure = %q, want empty (caller downgrades)", got)
	}
}

// C5b: HTTP 200 + business code != 0 (e.g. permission denied) -> "" + stderr warning.
func TestFetchFolderChildrenTree_BusinessError(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body:   map[string]interface{}{"code": 14009, "msg": "dlp deny"},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	if got != "" {
		t.Fatalf("fetchFolderChildrenTree() on business error = %q, want empty", got)
	}
}

// C6: genuinely empty folder (items empty, all_count 0) keeps child_count="0"
// instead of degrading to the pre-expand single-line output.
func TestFetchFolderChildrenTree_EmptyFolder(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":     []interface{}{},
				"all_count": float64(0),
			},
		},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	want := `<folder key="fld_root" name="x" child_count="0"/>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() empty folder = %q, want %q", got, want)
	}
}

// Prefetch: multiple folder messages in one page -> concurrent prefetch returns
// a cache keyed by message_id; Convert reuses it without a second GET.
func TestPrefetchFolderChildren(t *testing.T) {
	// roundtrip runtime (static token resolver): concurrent prefetch fan-out
	// must not trip the credential resolver under -race, same as merge tests.
	rt := newBotConvertlibRuntime(t, folderChildrenRoundTrip(false))
	rawItems := []interface{}{
		map[string]interface{}{"msg_type": "folder", "message_id": "om_100",
			"body": map[string]interface{}{"content": `{"file_key":"fld_om_100","file_name":"d1"}`}},
		map[string]interface{}{"msg_type": "folder", "message_id": "om_200",
			"body": map[string]interface{}{"content": `{"file_key":"fld_om_200","file_name":"d2"}`}},
		map[string]interface{}{"msg_type": "text", "message_id": "om_300"}, // non-folder ignored
	}
	cache := PrefetchFolderChildren(rt, rawItems)
	if len(cache) != 2 {
		t.Fatalf("PrefetchFolderChildren() = %d entries, want 2 (om_100/om_200)", len(cache))
	}
	for _, mid := range []string{"om_100", "om_200"} {
		xml, ok := cache[mid]
		if !ok || !strings.Contains(xml, `<folder key="fld_`+mid) || !strings.Contains(xml, `name="d`) {
			t.Fatalf("cache[%s] = %q, want expanded folder XML", mid, xml)
		}
	}
}

// Convert fast path: cached FolderChildren XML is reused (no second stub needed).
func TestFolderConverter_ConvertUsesPrefetchCache(t *testing.T) {
	rt, _ := folderTestRuntime(t)
	ctx := &ConvertContext{
		RawContent:     `{"file_key":"fld_x","file_name":"Docs"}`,
		MessageID:      "om_9",
		Runtime:        rt,
		FolderChildren: map[string]string{"om_9": `<folder key="fld_x" name="Docs" child_count="1"><file key="f1" name="a.pdf"/></folder>`},
	}
	got := (folderConverter{}).Convert(ctx)
	if got != `<folder key="fld_x" name="Docs" child_count="1"><file key="f1" name="a.pdf"/></folder>` {
		t.Fatalf("Convert() = %q, want cached XML (fast path)", got)
	}
}

// Convert-level inline expansion: folderConverter.Convert issues the GET when no
// prefetch cache is present, then renders the expanded XML (regression guard so
// removing the Convert-level call fails this test).
func TestFolderConverter_ConvertInline(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/im/v1/files/fld_root/folder?recursive=false&srcid=om_123&srctype=message",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":     []interface{}{map[string]interface{}{"file_key": "f1", "name": "a.pdf", "is_folder": false}},
			"all_count": float64(1),
		}},
	})
	ctx := &ConvertContext{RawContent: `{"file_key":"fld_root","file_name":"Docs"}`, MessageID: "om_123", Runtime: rt}
	got := (folderConverter{}).Convert(ctx)
	if !strings.Contains(got, `<folder key="fld_root" name="Docs" child_count="1">`) || !strings.Contains(got, `<file key="f1" name="a.pdf"/>`) {
		t.Fatalf("Convert() inline = %q, want expanded XML", got)
	}
}

// Convert-level fallback: GET failure (no stub) -> single-line tag + stderr warning.
func TestFolderConverter_ConvertInlineFallback(t *testing.T) {
	rt, _ := folderTestRuntime(t)
	ctx := &ConvertContext{RawContent: `{"file_key":"fld_root","file_name":"Docs"}`, MessageID: "om_123", Runtime: rt}
	got := (folderConverter{}).Convert(ctx)
	want := `<folder key="fld_root" name="Docs"/>`
	if got != want {
		t.Fatalf("Convert() fallback = %q, want %q", got, want)
	}
	if errOut := rt.IO().ErrOut.(*bytes.Buffer).String(); !strings.Contains(errOut, "folder_fetch_failed") {
		t.Fatalf("Convert() failure stderr = %q, want folder_fetch_failed warning", errOut)
	}
}

// Post message with a folder attachment expands one level (same fetch path as
// folder messages) — regression guard for renderPostAttachments.
func TestPostConverter_FolderAttachmentExpansion(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/im/v1/files/fld_att/folder?recursive=false&srcid=om_p&srctype=message",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":     []interface{}{map[string]interface{}{"file_key": "c1", "name": "sub.png", "is_folder": false}},
			"all_count": float64(1),
		}},
	})
	post := `{"zh_cn":{"text":"hi"},"files":[{"file_key":"fld_att","file_name":"assets","is_folder":true},{"file_key":"f1","file_name":"a.pdf"}]}`
	ctx := &ConvertContext{RawContent: post, MessageID: "om_p", Runtime: rt}
	got := (postConverter{}).Convert(ctx)
	if !strings.Contains(got, `<folder key="fld_att" name="assets" child_count="1">`) ||
		!strings.Contains(got, `<file key="c1" name="sub.png"/>`) ||
		!strings.Contains(got, `<file key="f1" name="a.pdf"/>`) {
		t.Fatalf("post Convert() = %q, want expanded folder attachment + file lines", got)
	}
}

// Transport/business failures must emit the stable folder_fetch_failed warning
// prefix on stderr (regression guard for the warning side effect).
func TestFolderChildrenTree_WarningOnStderr(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: folderURL, Body: map[string]interface{}{"code": 14009, "msg": "dlp deny"}})
	fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	errOut := rt.IO().ErrOut.(*bytes.Buffer).String()
	// stable warning prefix; the payload (err text or code) may vary by failure path
	if !strings.Contains(errOut, "folder_fetch_failed: fld_root:") {
		t.Fatalf("stderr = %q, want folder_fetch_failed warning for fld_root", errOut)
	}
}

// Prefetch failure sentinel (""): Convert degrades to the single-line tag and
// does NOT issue an inline retry (no stderr warning, no extra GET) — prevents
// page-wide failures (missing scope / DLP) from doubling every folder request.
func TestFolderConverter_PrefetchFailureSkipsInlineRetry(t *testing.T) {
	rt, _ := folderTestRuntime(t)
	ctx := &ConvertContext{
		RawContent:     `{"file_key":"fld_root","file_name":"Docs"}`,
		MessageID:      "om_fail",
		Runtime:        rt,
		FolderChildren: map[string]string{"om_fail": ""}, // prefetch tried & failed
	}
	got := (folderConverter{}).Convert(ctx)
	if got != `<folder key="fld_root" name="Docs"/>` {
		t.Fatalf("Convert() failure sentinel = %q, want single-line degrade", got)
	}
	if errOut := rt.IO().ErrOut.(*bytes.Buffer).String(); errOut != "" {
		t.Fatalf("Convert() sentinel stderr = %q, want empty (no inline retry)", errOut)
	}
}

// PrefetchFolderChildren records failures as "" sentinels (not absent keys), so
// the render loop never retries inline after a failed prefetch.
func TestPrefetchFolderChildren_FailureSentinel(t *testing.T) {
	rt := newBotConvertlibRuntime(t, folderChildrenRoundTrip(true)) // every fetch fails
	rawItems := []interface{}{
		map[string]interface{}{"msg_type": "folder", "message_id": "om_f1",
			"body": map[string]interface{}{"content": `{"file_key":"fld_f1","file_name":"d1"}`}},
		map[string]interface{}{"msg_type": "folder", "message_id": "om_f2",
			"body": map[string]interface{}{"content": `{"file_key":"fld_f2","file_name":"d2"}`}},
	}
	cache := PrefetchFolderChildren(rt, rawItems)
	if cache["om_f1"] != "" || cache["om_f2"] != "" {
		t.Fatalf("PrefetchFolderChildren() failures = %#v, want both \"\" sentinels", cache)
	}
	errOut := rt.IO().ErrOut.(*bytes.Buffer).String()
	if strings.Count(errOut, "folder_fetch_failed") != 2 {
		t.Fatalf("stderr warnings = %q, want 2 folder_fetch_failed (one per prefetch)", errOut)
	}
}

// Post messages with folder attachments are picked up by PrefetchFolderChildren
// (cache key message_id + folder key); the post converter reuses the cached XML
// instead of an inline GET.
func TestPostFolderAttachmentPrefetch(t *testing.T) {
	rt := newBotConvertlibRuntime(t, folderChildrenRoundTrip(false))
	raw := map[string]interface{}{
		"msg_type": "post", "message_id": "om_p1",
		"body": map[string]interface{}{"content": `{"zh_cn":{"text":"hi"},"files":[{"file_key":"fld_p1","file_name":"assets","is_folder":true}]}`},
	}
	cache := PrefetchFolderChildren(rt, []interface{}{raw})
	xml, ok := cache["om_p1\x00fld_p1"]
	if !ok || !strings.Contains(xml, `<folder key="fld_p1" name="assets" child_count="1">`) {
		t.Fatalf("PrefetchFolderChildren() post attachment = %q (ok=%v), want cached expansion", xml, ok)
	}
	// Render via postConverter — should reuse the cache (stub would only answer once).
	ctx := &ConvertContext{RawContent: raw["body"].(map[string]interface{})["content"].(string), MessageID: "om_p1", Runtime: rt, FolderChildren: cache}
	got := (postConverter{}).Convert(ctx)
	if !strings.Contains(got, `<folder key="fld_p1" name="assets" child_count="1">`) || !strings.Contains(got, `<file key="f1" name="a.pdf"/>`) {
		t.Fatalf("post Convert() with prefetch = %q, want cached folder expansion", got)
	}
}

// Server returns all_count > 0 but no items -> self-closing folder tag with
// child_count + has_more (no odd empty open/close pair).
func TestFetchFolderChildrenTree_NoItemsButAllCount(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    folderURL,
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{}, "all_count": float64(3)}},
	})
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	want := `<folder key="fld_root" name="x" child_count="3" has_more="true"/>`
	if got != want {
		t.Fatalf("fetchFolderChildrenTree() no-items-all-count = %q, want %q", got, want)
	}
}

// code:0 without a data field -> "" (degrade) plus an "empty data" stderr
// warning (not a misleading "<nil>").
func TestFetchFolderChildrenTree_EmptyData(t *testing.T) {
	rt, reg := folderTestRuntime(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: folderURL, Body: map[string]interface{}{"code": 0}})
	got := fetchFolderChildrenTree(rt, "fld_root", "x", "om_123")
	if got != "" {
		t.Fatalf("fetchFolderChildrenTree() empty data = %q, want empty", got)
	}
	errOut := rt.IO().ErrOut.(*bytes.Buffer).String()
	if !strings.Contains(errOut, "folder_fetch_failed: fld_root: empty data") {
		t.Fatalf("stderr = %q, want 'empty data' warning", errOut)
	}
}
