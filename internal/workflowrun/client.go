// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package workflowrun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
)

// WorkflowRunClient isolates the three proposed read-only APIs.
type WorkflowRunClient interface {
	Fetch(context.Context, string, Query) (Page, error)
}

// RealClient deliberately cannot reach a production endpoint.
type RealClient struct{}

func (RealClient) Fetch(context.Context, string, Query) (Page, error) {
	// TODO: implement only after production contracts T1–T14 are approved, using RuntimeContext.
	return Page{}, errs.NewValidationError(errs.SubtypeFailedPrecondition, "production workflow log APIs are not available; use mock fixtures")
}

type Client struct {
	HTTP *http.Client
	URL  string
	Wait func(context.Context, time.Duration) error
}

func NewClient(f *Fixture, endpoint string) (*Client, error) {
	c := &Client{Wait: wait}
	if endpoint == "" {
		c.URL = "http://workflow-mock.invalid"
		c.HTTP = &http.Client{Transport: f}
		return c, nil
	}
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "mock URL must be an HTTP loopback origin").WithParam("--mock-url")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "mock URL requires a literal loopback IP").WithParam("--mock-url")
	}
	c.URL = strings.TrimRight(endpoint, "/")
	// This client speaks only the local mock protocol: no credentials, proxies,
	// cookies, redirects or resolver-owned production hosts.
	c.HTTP = &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return c, nil
}
func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func Endpoint(plane string, q Query) string {
	base := "/open-apis/base/v3/bases/" + url.PathEscape(q.Base)
	if plane == Buttons {
		return base + "/workflow-button-logs"
	}
	if q.Workflow == "" {
		return base + "/workflow-runs"
	}
	p := base + "/workflows/" + url.PathEscape(q.Workflow) + "/runs"
	if plane == Details {
		p += "/" + url.PathEscape(q.Run)
	}
	return p
}
func (c *Client) Fetch(ctx context.Context, plane string, q Query) (Page, error) {
	wire, _ := json.Marshal(q)
	v := url.Values{"query": {string(wire)}}
	for attempt := 0; attempt < 3; attempt++ {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+Endpoint(plane, q)+"?"+v.Encode(), nil)
		if e != nil {
			return Page{}, errs.NewInternalError(errs.SubtypeSDKError, "cannot construct mock request").WithCause(e)
		}
		resp, e := c.HTTP.Do(req)
		if e != nil {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkTransport, "mock request failed").WithCause(e)
		}
		b, e := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
		resp.Body.Close()
		if e != nil {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkTransport, "cannot read mock response").WithCause(e)
		}
		if len(b) > 8*1024*1024 {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "mock response exceeds 8 MiB")
		}
		if resp.StatusCode == 403 {
			return Page{Plane: plane, Result: "permission_denied", Coverage: Unverified}, nil
		}
		if resp.StatusCode >= 500 {
			if attempt < 2 {
				if e := c.Wait(ctx, time.Duration(1<<attempt)*10*time.Millisecond); e != nil {
					return Page{}, errs.NewNetworkError(errs.SubtypeNetworkTransport, "mock request cancelled").WithCause(e)
				}
				continue
			}
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkServer, "mock request failed after bounded retries")
		}
		if resp.StatusCode != 200 {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "mock endpoint returned HTTP %d", resp.StatusCode)
		}
		var meta struct {
			HasMore *bool  `json:"has_more"`
			Result  string `json:"result"`
		}
		if json.Unmarshal(b, &meta) != nil || meta.HasMore == nil || meta.Result == "" {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "mock page lacks explicit pagination/result metadata")
		}
		var p Page
		if e = json.Unmarshal(b, &p); e != nil {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "invalid mock page").WithCause(e)
		}
		if p.Plane != plane || p.Window != q.Window.Wire() {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "mock page does not match requested plane/window")
		}
		if p.HasMore && (p.Token == "" || p.Token == q.Token) {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "non-advancing mock cursor")
		}
		if p.Coverage != Guaranteed && p.Coverage != Expired {
			p.Coverage = Unverified
		}
		if q.PageSize > 0 && len(p.Items) > q.PageSize {
			return Page{}, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "mock page exceeds requested record budget")
		}
		return p, nil
	}
	return Page{}, errs.NewInternalError(errs.SubtypeSDKError, "unreachable mock retry state")
}

type Event struct {
	Item      Item   `json:"item"`
	NodeID    string `json:"node_id"`
	Key       string `json:"button_key"`
	FlowID    int64  `json:"flow_inst_id"`
	VisibleAt int64  `json:"visible_at"`
}
type Fault struct {
	Plane    string `json:"plane"`
	Start    *int64 `json:"window_start,omitempty"`
	Run      string `json:"run_id,omitempty"`
	Ref      string `json:"button_ref,omitempty"`
	Token    string `json:"page_token,omitempty"`
	HTTP     int    `json:"http_status,omitempty"`
	Result   string `json:"result,omitempty"`
	Coverage string `json:"coverage,omitempty"`
	Count    int    `json:"count"`
}
type Fixture struct {
	Caps     Capabilities      `json:"capabilities"`
	PageSize int               `json:"page_size"`
	Coverage string            `json:"coverage"`
	Runs     []Item            `json:"runs"`
	Details  map[string][]Item `json:"details"`
	Events   []Event           `json:"events"`
	Faults   []Fault           `json:"faults"`
	mu       sync.Mutex
	Requests []Query `json:"-"`
}

func EmptyFixture() *Fixture {
	return &Fixture{Caps: Capabilities{true, true, ResponseField}, PageSize: 100, Coverage: Guaranteed, Details: map[string][]Item{}}
}
func (f *Fixture) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var q Query
	e := json.Unmarshal([]byte(req.URL.Query().Get("query")), &q)
	if e != nil {
		return nil, e
	}
	plane := Runs
	if strings.HasSuffix(req.URL.Path, "workflow-button-logs") {
		plane = Buttons
	} else if q.Run != "" {
		plane = Details
	}
	if req.Method != http.MethodGet || req.URL.EscapedPath() != Endpoint(plane, q) || req.Header.Get("Authorization") != "" {
		return response(req, 400, Page{}), nil
	}
	f.Requests = append(f.Requests, q)
	p := Page{Plane: plane, Window: q.Window.Wire(), Coverage: f.Coverage, Result: "ok", Caps: f.Caps, Items: []Item{}}
	if plane == Runs && q.Workflow == "" && !f.Caps.BaseAggregate {
		return response(req, 400, p), nil
	}
	for n := range f.Faults {
		x := &f.Faults[n]
		if x.Count == 0 || x.Plane != plane || (x.Start != nil && *x.Start != q.Window.Start) || (x.Run != "" && x.Run != q.Run) || x.Token != q.Token || (x.Ref != "" && (q.Group == nil || x.Ref != q.Group.Ref)) {
			continue
		}
		if x.Count > 0 {
			x.Count--
		}
		if x.HTTP != 0 {
			return response(req, x.HTTP, p), nil
		}
		p.Result = x.Result
		if x.Coverage != "" {
			p.Coverage = x.Coverage
		}
		if p.Result == "data_expired" {
			p.Coverage = Expired
			p.Uncovered = [][2]string{q.Window.Wire()}
		}
		return response(req, 200, p), nil
	}
	switch plane {
	case Runs:
		for _, i := range f.Runs {
			if i.StartTime < q.Window.Start || i.StartTime >= q.Window.End || (q.Workflow != "" && i.WorkflowID != q.Workflow) || (q.Status != "" && q.Status != "all" && i.Status != q.Status) || i.Duration < q.MinDuration || (q.MaxDuration > 0 && i.Duration > q.MaxDuration) || (len(q.Editors) > 0 && !slices.Contains(q.Editors, i.EditorID)) {
				continue
			}
			p.Items = append(p.Items, i)
		}
		sort.Slice(p.Items, func(a, b int) bool {
			x, y := p.Items[a], p.Items[b]
			if x.StartTime == y.StartTime {
				return x.RunID < y.RunID
			}
			return x.StartTime < y.StartTime
		})
	case Details:
		for _, run := range f.Runs {
			if run.RunID == q.Run {
				v := run.StartTime
				p.RunStartedAt = &v
				break
			}
		}
		p.Items = append(p.Items, f.Details[q.Run]...)
		if !q.IncludePayload {
			for n := range p.Items {
				stripPayload(&p.Items[n])
			}
		}
	case Buttons:
		if q.Group == nil {
			return response(req, 400, p), nil
		}
		g := q.Group
		for _, ev := range f.Events {
			i := ev.Item
			if i.RunID != q.Run || ev.NodeID != g.NodeID || ev.Key != g.Key || i.CreatedTime < q.Window.Start || i.CreatedTime >= q.Window.End || ev.VisibleAt > q.CollectedAt {
				continue
			}
			if g.Legacy {
				if ev.FlowID != 0 {
					continue
				}
			} else if g.FlowID != nil {
				if ev.FlowID != *g.FlowID && !(f.Caps.Attribution == LegacyCompat && ev.FlowID == 0) {
					continue
				}
			}
			i.NodeID = ev.NodeID
			i.ButtonKey = ev.Key
			i.ButtonRef = g.Ref
			if f.Caps.Attribution == ResponseField {
				v := ev.FlowID
				i.FlowID = &v
			} else {
				i.FlowID = nil
			}
			p.Items = append(p.Items, i)
		}
		sort.Slice(p.Items, func(a, b int) bool {
			x, y := p.Items[a], p.Items[b]
			if x.CreatedTime == y.CreatedTime {
				return x.EventID < y.EventID
			}
			return x.CreatedTime < y.CreatedTime
		})
		if q.Direction == "backward" {
			slices.Reverse(p.Items)
		}
	}
	// Cursor is opaque outside the mock adapter. The production compound cursor
	// (start_time/run_id or created_time/event_id) remains a contract proposal.
	offset := 0
	if q.Token != "" {
		offset, e = strconv.Atoi(strings.TrimPrefix(q.Token, "mock:"))
		if e != nil || offset < 0 || offset > len(p.Items) {
			return response(req, 400, p), nil
		}
	}
	size := f.PageSize
	if size <= 0 {
		size = 100
	}
	if q.PageSize > 0 && q.PageSize < size {
		size = q.PageSize
	}
	end := offset + size
	if end < len(p.Items) {
		p.HasMore = true
		p.Token = "mock:" + strconv.Itoa(end)
	} else {
		end = len(p.Items)
	}
	p.Items = p.Items[offset:end]
	return response(req, 200, p), nil
}
func stripPayload(i *Item) {
	i.Payload = nil
	i.Children = append([]Item(nil), i.Children...)
	for n := range i.Children {
		stripPayload(&i.Children[n])
	}
}
func response(req *http.Request, status int, p Page) *http.Response {
	b, _ := json.Marshal(p)
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(b)), Header: http.Header{"Content-Type": {"application/json"}}, Request: req}
}
