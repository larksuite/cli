package doc

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
)

type DocReadOptions struct {
	DocumentHost       string
	Input              string
	AllowedHosts       []string
	Identity           string
	Full               bool
	MaxPages, MaxCells int
}
type DocumentMaterial struct {
	bodyRead     bool
	Content      string                   `json:"content"`
	ContentMode  string                   `json:"content_mode"`
	ContentScope string                   `json:"content_scope"`
	Complete     bool                     `json:"complete"`
	Identity     string                   `json:"identity"`
	StartedAt    time.Time                `json:"started_at"`
	FinishedAt   time.Time                `json:"finished_at"`
	Revision     interface{}              `json:"revision,omitempty"`
	Snapshot     string                   `json:"snapshot"`
	Sheets       []map[string]interface{} `json:"sheets,omitempty"`
	Unexpanded   []map[string]string      `json:"unexpanded,omitempty"`
	Unsupported  []string                 `json:"unsupported_content"`
}
type DocumentGET func(context.Context, string, map[string]interface{}) (map[string]interface{}, error)

var resourceToken = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func documentInput(input string, hosts []string) (kind, token string, err error) {
	if resourceToken.MatchString(input) {
		return "docx", input, nil
	}
	parsed, e := url.Parse(input)
	if e != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid document URL")
	}
	allowed := false
	for _, host := range hosts {
		if strings.EqualFold(parsed.Host, host) {
			allowed = true
		}
	}
	if !allowed {
		return "", "", fmt.Errorf("document host outside configured tenant")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unsupported document URL")
	}
	kind, token = parts[len(parts)-2], parts[len(parts)-1]
	if (kind != "docx" && kind != "wiki") || !resourceToken.MatchString(token) {
		return "", "", fmt.Errorf("unsupported document type")
	}
	return kind, token, nil
}
func ReadDocument(ctx context.Context, opts DocReadOptions, get DocumentGET) (result *DocumentMaterial, err error) {
	get = retryDocumentGET(get)
	result = &DocumentMaterial{ContentMode: "raw", ContentScope: "document_body_only", Identity: opts.Identity, StartedAt: time.Now().UTC(), Snapshot: "paged resources are not an atomic cross-resource snapshot", Unsupported: []string{"image OCR", "attachment contents", "charts and formula semantics", "synced blocks and nested document recursion"}}
	defer func() { result.FinishedAt = time.Now().UTC() }()
	hosts := append([]string(nil), opts.AllowedHosts...)
	if opts.DocumentHost != "" {
		host, e := exactDocumentHost(opts.DocumentHost)
		if e != nil {
			return result, e
		}
		hosts = append(hosts, host)
	}
	kind, token, err := documentInput(opts.Input, hosts)
	if err != nil {
		return result, err
	}
	if kind == "wiki" {
		data, e := get(ctx, "/open-apis/wiki/v2/spaces/get_node", map[string]interface{}{"token": token})
		if e != nil {
			return result, e
		}
		node, _ := data["node"].(map[string]interface{})
		if node["obj_type"] != "docx" {
			return result, fmt.Errorf("unsupported wiki object type")
		}
		token, _ = node["obj_token"].(string)
		if !resourceToken.MatchString(token) {
			return result, fmt.Errorf("invalid wiki document token")
		}
	}
	base := "/open-apis/docx/v1/documents/" + url.PathEscape(token)
	before, metadataErr := get(ctx, base, nil)
	if metadataErr == nil {
		if doc, ok := before["document"].(map[string]interface{}); ok {
			result.Revision = doc["revision_id"]
		}
	}
	raw, e := get(ctx, base+"/raw_content", nil)
	if e != nil {
		return result, e
	}
	content, ok := raw["content"].(string)
	if !ok {
		return result, fmt.Errorf("invalid raw content response")
	}
	result.Content = content
	result.bodyRead = true
	result.Complete = true
	if !opts.Full {
		return result, nil
	}
	result.ContentMode = "materials"
	result.ContentScope = "document_body_and_recognized_embedded_sheet_values"
	if metadataErr != nil {
		result.Complete = false
		result.Unexpanded = append(result.Unexpanded, map[string]string{"resource": token, "reason": "document revision unavailable", "identity": opts.Identity})
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 100
	}
	cursor := ""
	visited := map[string]bool{}
	sheetsVisited := map[string]bool{}
	cellsRemaining := opts.MaxCells
	if cellsRemaining <= 0 {
		cellsRemaining = 100000
	}
	for page := 0; page < maxPages; page++ {
		params := map[string]interface{}{"page_size": 500}
		if cursor != "" {
			params["page_token"] = cursor
		}
		data, e := get(ctx, base+"/blocks", params)
		if e != nil {
			result.incomplete(token, "blocks request failed")
			break
		}
		items, ok := data["items"].([]interface{})
		if !ok {
			result.incomplete(token, "invalid blocks response")
			break
		}
		for _, item := range items {
			block, ok := item.(map[string]interface{})
			if !ok {
				result.incomplete(token, "invalid block")
				continue
			}
			sheet, ok := block["sheet"].(map[string]interface{})
			if !ok {
				continue
			}
			sheetToken, _ := sheet["token"].(string)
			if !resourceToken.MatchString(sheetToken) {
				result.incomplete(token, "unrecognized embedded sheet reference")
				continue
			}
			if sheetsVisited[sheetToken] {
				continue
			}
			sheetsVisited[sheetToken] = true
			result.readEmbeddedSheet(ctx, get, sheetToken, &cellsRemaining)
		}
		more, _ := data["has_more"].(bool)
		if !more {
			break
		}
		next, _ := data["page_token"].(string)
		if next == "" || visited[next] {
			result.incomplete(token, "invalid or cyclic blocks cursor")
			break
		}
		visited[next] = true
		cursor = next
		if page == maxPages-1 {
			result.incomplete(token, "blocks page limit")
		}
	}
	after, e := get(ctx, base, nil)
	if e != nil {
		result.incomplete(token, "document revision unavailable after read")
	} else if doc, ok := after["document"].(map[string]interface{}); ok && result.Revision != nil && fmt.Sprint(doc["revision_id"]) != fmt.Sprint(result.Revision) {
		result.incomplete(token, "document changed during paginated read")
	}
	if !result.Complete {
		return result, fmt.Errorf("document_materials_incomplete")
	}
	return result, nil
}
func (r *DocumentMaterial) incomplete(resource, reason string) {
	r.Complete = false
	r.Unexpanded = append(r.Unexpanded, map[string]string{"resource": resource, "reason": reason, "identity": r.Identity})
}

func numeric(value interface{}) (int, bool) {
	n, e := strconv.Atoi(fmt.Sprint(value))
	return n, e == nil
}
func spreadsheetColumn(n int) string {
	out := ""
	for n > 0 {
		n--
		out = string(rune('A'+n%26)) + out
		n /= 26
	}
	return out
}
func (r *DocumentMaterial) readEmbeddedSheet(ctx context.Context, get DocumentGET, token string, budget *int) {
	// Private-server evidence: block.sheet.token = spreadsheet_token + "_" + sheet_id.
	// Legacy references without a separator retain their established whole-spreadsheet behavior.
	reference := token
	referencedSheet := ""
	if split := strings.LastIndex(token, "_"); split >= 0 {
		referencedSheet = token[split+1:]
		token = token[:split]
		if !resourceToken.MatchString(token) || !resourceToken.MatchString(referencedSheet) {
			r.incomplete(reference, "unrecognized compound sheet reference")
			return
		}
	}

	prefix := "/open-apis/sheets/v3/spreadsheets/" + url.PathEscape(token)
	data, e := get(ctx, prefix+"/sheets/query", nil)
	if e != nil {
		r.incomplete(token, "embedded sheet metadata unavailable")
		return
	}
	sheets, ok := data["sheets"].([]interface{})
	if !ok {
		r.incomplete(token, "invalid embedded sheet metadata")
		return
	}
	foundReference := false
	for _, raw := range sheets {
		sheet, ok := raw.(map[string]interface{})
		if !ok {
			r.incomplete(token, "invalid sheet")
			continue
		}
		id, _ := sheet["sheet_id"].(string)
		if referencedSheet != "" && id != referencedSheet {
			continue
		}
		foundReference = true
		if !resourceToken.MatchString(id) {
			r.incomplete(token, "unrecognized sheet ID")
			continue
		}
		grid, _ := sheet["grid_properties"].(map[string]interface{})
		rows, rok := numeric(grid["row_count"])
		cols, cok := numeric(grid["column_count"])
		if !rok || !cok || rows < 0 || cols < 0 {
			r.incomplete(token+"/"+id, "sheet dimensions unavailable")
			continue
		}
		if kind, _ := sheet["resource_type"].(string); kind != "" && kind != "sheet" {
			r.incomplete(token+"/"+id, "unsupported sheet resource type")
			continue
		}
		material := map[string]interface{}{"spreadsheet_token": token, "sheet_id": id, "title": sheet["title"], "row_count": rows, "column_count": cols, "value_semantics": "FormattedValue; formulas and computed semantics are not source formulas", "ranges": []interface{}{}}
		ranges := []interface{}{}
		r.Sheets = append(r.Sheets, material)
		if rows == 0 || cols == 0 {
			continue
		}
		if cols > *budget || cols > 10000 {
			r.incomplete(token+"/"+id, "cell limit")
			continue
		}
		for start := 1; start <= rows; {
			chunk := 100
			if rows-start+1 < chunk {
				chunk = rows - start + 1
			}
			if *budget/cols < chunk {
				chunk = *budget / cols
			}
			if chunk <= 0 {
				r.incomplete(token+"/"+id, "cell limit")
				break
			}
			end := start + chunk - 1
			cellRange := fmt.Sprintf("%s!A%d:%s%d", id, start, spreadsheetColumn(cols), end)
			response, e := get(ctx, "/open-apis/sheets/v2/spreadsheets/"+url.PathEscape(token)+"/values/"+url.PathEscape(cellRange), map[string]interface{}{"valueRenderOption": "FormattedValue"})
			if e != nil {
				r.incomplete(token+"/"+cellRange, "sheet values unavailable")
				break
			}
			valueRange, ok := response["valueRange"].(map[string]interface{})
			if !ok {
				r.incomplete(token+"/"+cellRange, "invalid sheet values")
				break
			}
			if reported, _ := valueRange["range"].(string); reported != "" && reported != cellRange {
				r.incomplete(token+"/"+cellRange, "sheet response range differs")
				break
			}
			ranges = append(ranges, map[string]interface{}{"range": cellRange, "values": valueRange["values"]})
			material["ranges"] = ranges
			*budget -= chunk * cols
			start = end + 1
		}
	}
	if referencedSheet != "" && !foundReference {
		r.incomplete(reference, "referenced sheet tab not found")
	}
}

func retryDocumentGET(base DocumentGET) DocumentGET {
	return func(ctx context.Context, path string, params map[string]interface{}) (map[string]interface{}, error) {
		for attempt := 0; ; attempt++ {
			result, err := base(ctx, path, params)
			if err == nil || attempt >= 2 {
				return result, err
			}
			retry := false
			delay := time.Duration(100+rand.IntN(100)) * time.Millisecond
			var diagnostic *output.ExitError
			if errors.As(err, &diagnostic) && diagnostic.Detail != nil {
				status := diagnostic.Detail.HTTPStatus
				retry = status == 429 || status >= 500
				if diagnostic.Detail.RetryAfterMS > 0 {
					delay = time.Duration(diagnostic.Detail.RetryAfterMS) * time.Millisecond
				}
			}
			var network net.Error
			if errors.As(err, &network) {
				retry = network.Timeout() || network.Temporary()
			}
			if !retry {
				return nil, err
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
	}
}

// exactDocumentHost grants only one DNS hostname for input parsing. API routing is unchanged.
func exactDocumentHost(host string) (string, error) {
	if len(host) > 253 || strings.TrimSpace(host) != host || host == "" {
		return "", fmt.Errorf("--document-host requires an exact hostname, without URL, port or wildcard")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid --document-host hostname")
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return "", fmt.Errorf("--document-host requires an exact hostname, without URL, port or wildcard")
			}
		}
	}
	return strings.ToLower(host), nil
}
