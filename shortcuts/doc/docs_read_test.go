package doc

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func TestReadDocumentRawScopeAndTenantURLBoundary(t *testing.T) {
	calls := 0
	get := func(_ context.Context, path string, _ map[string]interface{}) (map[string]interface{}, error) {
		calls++
		if strings.HasSuffix(path, "/raw_content") {
			return map[string]interface{}{"content": "body"}, nil
		}
		return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
	}
	result, err := ReadDocument(context.Background(), DocReadOptions{Input: "https://tenant.test/docx/doc1", AllowedHosts: []string{"tenant.test"}, Identity: "bot"}, get)
	if err != nil || !result.Complete || result.ContentScope != "document_body_only" || result.Content != "body" {
		t.Fatalf("wrong raw result: %#v %v", result, err)
	}
	before := calls
	_, err = ReadDocument(context.Background(), DocReadOptions{Input: "https://foreign.test/docx/doc1", AllowedHosts: []string{"tenant.test"}}, get)
	if err == nil || calls != before {
		t.Fatal("foreign input reached API")
	}
}

func TestReadMaterialsPaginatesBlocksAndMarksChangingRevisionPartial(t *testing.T) {
	metadataCalls, blockCalls := 0, 0
	get := func(_ context.Context, path string, params map[string]interface{}) (map[string]interface{}, error) {
		if strings.HasSuffix(path, "/raw_content") {
			return map[string]interface{}{"content": "body"}, nil
		}
		if strings.HasSuffix(path, "/blocks") {
			blockCalls++
			if blockCalls == 1 {
				return map[string]interface{}{"items": []interface{}{}, "has_more": true, "page_token": "next"}, nil
			}
			if params["page_token"] != "next" {
				t.Fatal("lost cursor")
			}
			return map[string]interface{}{"items": []interface{}{}, "has_more": false}, nil
		}
		metadataCalls++
		return map[string]interface{}{"document": map[string]interface{}{"revision_id": metadataCalls}}, nil
	}
	result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc", Full: true, Identity: "bot"}, get)
	if err == nil || result.Complete || blockCalls != 2 {
		t.Fatalf("changing/paginated read claimed complete: %#v %v blocks=%d", result, err, blockCalls)
	}
}

func TestReadMaterialsExpandsDistinctSheetsAndPreservesCoordinates(t *testing.T) {
	valueCalls := 0
	get := func(_ context.Context, path string, params map[string]interface{}) (map[string]interface{}, error) {
		switch {
		case strings.HasSuffix(path, "/raw_content"):
			return map[string]interface{}{"content": "body"}, nil
		case strings.HasSuffix(path, "/blocks"):
			sheet := map[string]interface{}{"sheet": map[string]interface{}{"token": "sheetToken"}}
			return map[string]interface{}{"items": []interface{}{sheet, sheet}, "has_more": false}, nil
		case strings.HasSuffix(path, "/sheets/query"):
			return map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"sheet_id": "sheet1", "title": "Budget", "grid_properties": map[string]interface{}{"row_count": 2, "column_count": 2}}}}, nil
		case strings.Contains(path, "/values/"):
			valueCalls++
			decoded, _ := url.PathUnescape(path)
			if !strings.HasSuffix(decoded, "sheet1!A1:B2") {
				t.Fatalf("wrong coordinates %s", path)
			}
			return map[string]interface{}{"valueRange": map[string]interface{}{"range": "sheet1!A1:B2", "values": []interface{}{[]interface{}{"name", "value"}, []interface{}{"cost", 7}}}}, nil
		default:
			return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
		}
	}
	result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc", Full: true, Identity: "bot"}, get)
	if err != nil || !result.Complete || valueCalls != 1 || len(result.Sheets) != 1 {
		t.Fatalf("sheet missing or duplicate: %#v %v", result, err)
	}
}

func TestReadDocumentRetriesOnlySafeTemporaryReads(t *testing.T) {
	rawCalls := 0
	get := func(_ context.Context, path string, _ map[string]interface{}) (map[string]interface{}, error) {
		if strings.HasSuffix(path, "/raw_content") {
			rawCalls++
			if rawCalls < 3 {
				failure := output.ErrNetwork("temporary")
				failure.Detail.HTTPStatus = 503
				return nil, failure
			}
			return map[string]interface{}{"content": "recovered"}, nil
		}
		return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
	}
	result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc"}, get)
	if err != nil || result.Content != "recovered" || rawCalls != 3 {
		t.Fatalf("temporary read not recovered: calls=%d err=%v", rawCalls, err)
	}
}

func TestReadMaterialsFailureAndCellLimitRemainExplicitlyPartial(t *testing.T) {
	for _, mode := range []string{"denied", "limit", "empty"} {
		t.Run(mode, func(t *testing.T) {
			get := func(_ context.Context, path string, _ map[string]interface{}) (map[string]interface{}, error) {
				switch {
				case strings.HasSuffix(path, "/raw_content"):
					return map[string]interface{}{"content": "body"}, nil
				case strings.HasSuffix(path, "/blocks"):
					return map[string]interface{}{"items": []interface{}{map[string]interface{}{"sheet": map[string]interface{}{"token": "sheetToken"}}}, "has_more": false}, nil
				case strings.HasSuffix(path, "/sheets/query"):
					if mode == "denied" {
						return nil, output.ErrAPI(1770032, "denied", nil)
					}
					rows := 2
					if mode == "empty" {
						rows = 0
					}
					return map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"sheet_id": "id", "grid_properties": map[string]interface{}{"row_count": rows, "column_count": 2}}}}, nil
				default:
					return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
				}
			}
			result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc", Full: true, Identity: "bot", MaxCells: 1}, get)
			if mode == "empty" {
				if err != nil || !result.Complete || len(result.Sheets) != 1 {
					t.Fatal("empty sheet confused with unreadable")
				}
				return
			}
			if err == nil || result.Complete || len(result.Unexpanded) == 0 || result.Content != "body" {
				t.Fatalf("missing partial contract: %#v %v", result, err)
			}
		})
	}
}
func TestWikiResolvesOnlySupportedDocxObject(t *testing.T) {
	for _, kind := range []string{"docx", "doc", "sheet"} {
		t.Run(kind, func(t *testing.T) {
			rawCalls := 0
			get := func(_ context.Context, path string, params map[string]interface{}) (map[string]interface{}, error) {
				if strings.HasSuffix(path, "/get_node") {
					if params["token"] != "wikiToken" {
						t.Fatal("wrong wiki input")
					}
					return map[string]interface{}{"node": map[string]interface{}{"obj_type": kind, "obj_token": "resolvedDoc"}}, nil
				}
				if strings.HasSuffix(path, "/raw_content") {
					rawCalls++
					return map[string]interface{}{"content": "resolved"}, nil
				}
				return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
			}
			_, err := ReadDocument(context.Background(), DocReadOptions{Input: "https://tenant.test/wiki/wikiToken", AllowedHosts: []string{"tenant.test"}}, get)
			if kind == "docx" {
				if err != nil || rawCalls != 1 {
					t.Fatal("wiki docx not resolved")
				}
			} else if err == nil || rawCalls != 0 {
				t.Fatal("unsupported wiki object treated as docx")
			}
		})
	}
}

func TestReadMaterialsReadsAllRangesAndStopsCyclicBlocks(t *testing.T) {
	for _, cycle := range []bool{false, true} {
		t.Run(fmt.Sprint(cycle), func(t *testing.T) {
			ranges, blocks := 0, 0
			get := func(_ context.Context, path string, params map[string]interface{}) (map[string]interface{}, error) {
				switch {
				case strings.HasSuffix(path, "/raw_content"):
					return map[string]interface{}{"content": "body"}, nil
				case strings.HasSuffix(path, "/blocks"):
					blocks++
					return map[string]interface{}{"items": []interface{}{map[string]interface{}{"sheet": map[string]interface{}{"token": "spreadsheet"}}}, "has_more": cycle, "page_token": "repeated"}, nil
				case strings.HasSuffix(path, "/sheets/query"):
					return map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"sheet_id": "id", "grid_properties": map[string]interface{}{"row_count": 201, "column_count": 1}}}}, nil
				case strings.Contains(path, "/values/"):
					ranges++
					return map[string]interface{}{"valueRange": map[string]interface{}{"values": []interface{}{}}}, nil
				default:
					return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
				}
			}
			result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc", Full: true}, get)
			if ranges != 3 {
				t.Fatalf("incomplete or repeated sheet ranges: %d", ranges)
			}
			if cycle {
				if err == nil || result.Complete || blocks != 2 {
					t.Fatal("cyclic blocks claimed complete")
				}
			} else if err != nil || !result.Complete {
				t.Fatalf("all ranges not complete: %v", err)
			}
		})
	}
}

func TestDocumentHostOverrideIsExactAndNeverAllowsURLOrWildcard(t *testing.T) {
	for _, override := range []string{"https://documents.tenant.test", "*.tenant.test", "documents.tenant.test:443", "documents.tenant.test/path", " documents.tenant.test", "documents..tenant.test"} {
		calls := 0
		_, err := ReadDocument(context.Background(), DocReadOptions{Input: "https://documents.tenant.test/docx/doc", DocumentHost: override}, func(context.Context, string, map[string]interface{}) (map[string]interface{}, error) {
			calls++
			return nil, nil
		})
		if err == nil || calls != 0 {
			t.Fatalf("unsafe document host reached API: %q calls=%d", override, calls)
		}
	}
	calls := 0
	_, err := ReadDocument(context.Background(), DocReadOptions{Input: "https://other.tenant.test/docx/doc", DocumentHost: "documents.tenant.test"}, func(context.Context, string, map[string]interface{}) (map[string]interface{}, error) {
		calls++
		return nil, nil
	})
	if err == nil || calls != 0 {
		t.Fatal("exact allowlist expanded to another host")
	}
}

func TestCompoundEmbeddedSheetReadsOnlyReferencedTabAndPreservesRichCells(t *testing.T) {
	rich := []interface{}{map[string]interface{}{"type": "text", "text": "验收"}, map[string]interface{}{"type": "mention", "text": "负责人"}, map[string]interface{}{"type": "attachment", "text": "附件"}}
	metadataCalls, valueCalls := 0, 0
	get := func(_ context.Context, path string, _ map[string]interface{}) (map[string]interface{}, error) {
		switch {
		case strings.HasSuffix(path, "/raw_content"):
			return map[string]interface{}{"content": "body"}, nil
		case strings.HasSuffix(path, "/blocks"):
			return map[string]interface{}{"items": []interface{}{map[string]interface{}{"sheet": map[string]interface{}{"token": "shtrExample_LEi0UM"}}}, "has_more": false}, nil
		case strings.Contains(path, "/sheets/query"):
			metadataCalls++
			if path != "/open-apis/sheets/v3/spreadsheets/shtrExample/sheets/query" {
				t.Fatalf("compound reference sent as spreadsheet token: %s", path)
			}
			return map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"sheet_id": "other", "grid_properties": map[string]interface{}{"row_count": 100, "column_count": 100}}, map[string]interface{}{"sheet_id": "LEi0UM", "title": "Referenced", "grid_properties": map[string]interface{}{"row_count": 6, "column_count": 5}}}}, nil
		case strings.Contains(path, "/values/"):
			valueCalls++
			decoded, _ := url.PathUnescape(path)
			if decoded != "/open-apis/sheets/v2/spreadsheets/shtrExample/values/LEi0UM!A1:E6" {
				t.Fatalf("read unrelated or wrong range: %s", decoded)
			}
			return map[string]interface{}{"valueRange": map[string]interface{}{"range": "LEi0UM!A1:E6", "values": []interface{}{[]interface{}{rich}}}}, nil
		default:
			return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
		}
	}
	result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc", Full: true}, get)
	if err != nil || !result.Complete || metadataCalls != 1 || valueCalls != 1 || len(result.Sheets) != 1 {
		t.Fatalf("compound sheet incomplete: err=%v metadata=%d values=%d sheets=%d", err, metadataCalls, valueCalls, len(result.Sheets))
	}
	ranges := result.Sheets[0]["ranges"].([]interface{})
	got := ranges[0].(map[string]interface{})["values"].([]interface{})[0].([]interface{})[0]
	if !reflect.DeepEqual(got, rich) {
		t.Fatal("rich sheet cells lost structure")
	}
}

func TestCompoundEmbeddedSheetMissingReferencedTabIsPartialWithoutOtherValues(t *testing.T) {
	values := 0
	get := func(_ context.Context, path string, _ map[string]interface{}) (map[string]interface{}, error) {
		switch {
		case strings.HasSuffix(path, "/raw_content"):
			return map[string]interface{}{"content": "retained"}, nil
		case strings.HasSuffix(path, "/blocks"):
			return map[string]interface{}{"items": []interface{}{map[string]interface{}{"sheet": map[string]interface{}{"token": "spreadsheet_missing"}}}, "has_more": false}, nil
		case strings.HasSuffix(path, "/sheets/query"):
			return map[string]interface{}{"sheets": []interface{}{map[string]interface{}{"sheet_id": "unrelated", "grid_properties": map[string]interface{}{"row_count": 1, "column_count": 1}}}}, nil
		case strings.Contains(path, "/values/"):
			values++
			return nil, nil
		default:
			return map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}, nil
		}
	}
	result, err := ReadDocument(context.Background(), DocReadOptions{Input: "doc", Full: true}, get)
	if err == nil || result.Complete || values != 0 || len(result.Unexpanded) == 0 || result.Content != "retained" {
		t.Fatalf("unknown reference fell back to unrelated tabs: values=%d err=%v", values, err)
	}
}
