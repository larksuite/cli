// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package workflowrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/larksuite/cli/errs"
)

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		values = append(values, value)
	}
	return values
}
func ValidateParams(p Params) error {
	if p.Query.Base == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "base token is required").WithParam("--base-token")
	}
	if p.Query.Window.End <= p.Query.Window.Start {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "end time must follow start time").WithParam("--end-time")
	}
	// Resource bound for this mock preflight, not a proposed retention period.
	if p.Query.Run == "" && uint64(p.Query.Window.End)-uint64(p.Query.Window.Start) > 10000*uint64(24*time.Hour/time.Millisecond) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "mock export exceeds 10000 windows")
	}
	if p.Query.MinDuration < 0 || p.Query.MaxDuration < 0 || (p.Query.MaxDuration > 0 && p.Query.MaxDuration < p.Query.MinDuration) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid duration bounds")
	}
	if p.Query.Status != "" && !slices.Contains(StatusNames, p.Query.Status) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid run status").WithParam("--status")
	}
	if p.FormatVersion != FormatVersion {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "unsupported export format version")
	}
	if len(p.Planes) == 0 || !slices.Contains(p.Planes, Runs) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "run selection plane is required")
	}
	for _, plane := range p.Planes {
		if plane != Runs && plane != Details && plane != Buttons {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown export plane")
		}
	}
	if slices.Contains(p.Planes, Buttons) && !slices.Contains(p.Planes, Details) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "button export requires detail indexes")
	}
	return nil
}
func NewManifest(p Params) *Manifest {
	m := &Manifest{Params: p, SelectionWindow: p.Query.Window, Selected: []Selected{}, Planes: map[string]PlaneState{}, Scope: "selected runs by instance start time; button events through each frozen collected_at; no per-recipient delivery results"}
	for _, plane := range p.Planes {
		m.Planes[plane] = PlaneState{State: "unknown", QueryRange: map[string]Window{}}
	}
	for n, w := range Windows(p.Query.Window) {
		q := p.Query
		q.Window = w
		q.Token = ""
		m.Jobs = append(m.Jobs, Job{ID: fmt.Sprintf("r%06d", n), Plane: Runs, Query: q})
	}
	return m
}
func Compatible(frozen Params, provided Params) error {
	a, _ := json.Marshal(frozen)
	b, _ := json.Marshal(provided)
	if string(a) != string(b) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "resume parameters conflict with frozen manifest").WithParam("--resume")
	}
	return nil
}

type Engine struct {
	Client WorkflowRunClient
	Now    func() time.Time
}

func (e Engine) now() int64 {
	if e.Now != nil {
		return e.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func normalize(plane string, q Query, p *Page, fetched int64, source string) {
	p.DataSource = source
	if plane == Details {
		p.Items = Flatten(p.Items)
	}
	for n := range p.Items {
		i := &p.Items[n]
		i.FetchedAt = fetched
		i.DataSource = source
		if plane == Details && i.SendResults == "" {
			i.SendResults = "unavailable"
		}
		if plane != Buttons {
			continue
		}
		switch p.Caps.Attribution {
		case ResponseField:
			if i.FlowID == nil {
				i.Iteration = "unknown"
				p.Caps.Attribution = LegacyCompat
			} else if *i.FlowID == 0 {
				i.Iteration = "legacy"
			} else {
				i.Iteration = strconv.FormatInt(*i.FlowID, 10)
			}
		case StrictLegacy:
			// Only a declared strict equality backend permits attribution from the
			// queried group. Legacy is a separate once-per-run/node/key query.
			if q.Group != nil && q.Group.Legacy {
				v := int64(0)
				i.FlowID = &v
				i.Iteration = "legacy"
			} else if q.Group != nil && q.Group.FlowID != nil {
				v := *q.Group.FlowID
				i.FlowID = &v
				i.Iteration = strconv.FormatInt(v, 10)
			} else {
				i.Iteration = "unknown"
				p.Caps.Attribution = LegacyCompat
			}
		default:
			i.FlowID = nil
			i.Iteration = "unknown"
		}
	}
}
func (e Engine) derive(s *Store, m *Manifest) error {
	cps, err := s.Checkpoints()
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, j := range m.Jobs {
		key := j.Plane + "/" + j.Query.Run
		if j.Query.Group != nil {
			key += "/" + j.Query.Group.Ref
		}
		existing[key] = true
	}
	selected := map[string]bool{}
	for _, r := range m.Selected {
		selected[r.RunID] = true
	}
	add := func(plane string, q Query) {
		key := plane + "/" + q.Run
		if q.Group != nil {
			key += "/" + q.Group.Ref
		}
		if existing[key] {
			return
		}
		existing[key] = true
		m.Jobs = append(m.Jobs, Job{ID: fmt.Sprintf("j%06d", len(m.Jobs)), Plane: plane, Query: q})
	}
	for _, cp := range cps {
		if !batchSourceMatches(cp.Batch, m.DataSource) {
			return errs.NewInternalError(errs.SubtypeFileIO, "batch data source conflicts with manifest")
		}
		b := cp.Batch
		if b.Page.Plane == Runs {
			for _, i := range b.Page.Items {
				if !selected[i.RunID] {
					m.Selected = append(m.Selected, Selected{i.RunID, i.WorkflowID, i.StartTime, b.Query.CollectedAt})
					selected[i.RunID] = true
				}
				if slices.Contains(m.Params.Planes, Details) {
					q := m.Params.Query
					q.Run = i.RunID
					q.Workflow = i.WorkflowID
					q.Token = ""
					q.Window = Window{i.StartTime, b.Query.CollectedAt}
					q.CollectedAt = b.Query.CollectedAt
					add(Details, q)
				}
			}
		}
		if b.Page.Plane == Details && slices.Contains(m.Params.Planes, Buttons) {
			for _, i := range b.Page.Items {
				for _, g := range i.Groups {
					q := b.Query
					q.Token = ""
					q.PageSize = 0
					q.Group = &g
					add(Buttons, q)
					// Compound key is confined to this adapter, never accepted as CLI input.
					legacy := Group{Ref: "legacy:" + g.NodeID + ":" + g.Key, NodeID: g.NodeID, Key: g.Key, Legacy: true}
					q.Group = &legacy
					add(Buttons, q)
				}
			}
		}
	}
	if slices.Contains(m.Params.Planes, Buttons) {
		state := m.Planes[Buttons]
		if state.QueryRange == nil {
			state.QueryRange = map[string]Window{}
		}
		for _, r := range m.Selected {
			state.QueryRange[r.RunID] = Window{r.StartTime, r.CollectedAt}
		}
		m.Planes[Buttons] = state
	}
	return nil
}
func Summarize(m *Manifest) {
	m.Reasons = nil
	m.Pending = []string{}
	m.Uncovered = nil
	m.Completeness = "complete"
	for _, plane := range m.Params.Planes {
		state := m.Planes[plane]
		state.State = "complete"
		for _, j := range m.Jobs {
			if j.Plane != plane {
				continue
			}
			if !j.Done {
				m.Pending = appendUnique(m.Pending, j.ID)
				state.State = "partial"
			}
			if j.Coverage != Guaranteed {
				state.State = "unknown"
				if j.Coverage == Expired {
					state.State = "partial"
				}
				m.Reasons = appendUnique(m.Reasons, "coverage_unverified")
			}
			for _, reason := range j.Reasons {
				m.Reasons = appendUnique(m.Reasons, reason)
				if state.State == "complete" {
					state.State = "partial"
				}
			}
			m.Uncovered = append(m.Uncovered, j.Uncovered...)
		}
		// No selected instances inherits run-window coverage, never infers button
		// retention from an empty index or absent jobs.
		if plane != Runs && len(m.Selected) == 0 {
			for _, j := range m.Jobs {
				if j.Plane == Runs && (j.Coverage != Guaranteed || !j.Done) {
					state.State = "unknown"
				}
			}
		}
		m.Planes[plane] = state
		if state.State != "complete" {
			m.Completeness = "partial"
		}
	}
	if slices.Contains(m.Reasons, "max_records_limit") || slices.Contains(m.Reasons, "server_clip") {
		m.Completeness = "truncated"
	}
}
func (e Engine) Export(ctx context.Context, s *Store, m *Manifest, maxRecords int) error {
	source := e.Client.DataSource()
	if m.DataSource == "" {
		m.DataSource = source
	} else if m.DataSource != source {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "resume data source conflicts with frozen manifest").WithParam("--resume")
	}
	if err := ValidateParams(m.Params); err != nil {
		return err
	}
	if err := e.derive(s, m); err != nil {
		return err
	}
	if err := s.SaveManifest(m); err != nil {
		return err
	}
	count := 0
	limited := false
	for index := 0; index < len(m.Jobs); index++ {
		j := &m.Jobs[index]
		if j.Done {
			continue
		}
		if err := s.SaveManifest(m); err != nil {
			return err
		}
		// Retryable reasons describe the latest attempt. Durable coverage and
		// terminal expiration remain in their checkpoints.
		persistent := []string{}
		for _, reason := range j.Reasons {
			if reason != "request_failed" && reason != "permission_denied" && reason != "cancelled" && reason != "max_records_limit" {
				persistent = appendUnique(persistent, reason)
			}
		}
		j.Reasons = persistent
		for !j.Done {
			if ctx.Err() != nil {
				j.Reasons = appendUnique(j.Reasons, "cancelled")
				Summarize(m)
				_ = s.SaveManifest(m)
				return errs.NewNetworkError(errs.SubtypeNetworkTransport, "export cancelled").WithCause(ctx.Err())
			}
			if j.Plane == Runs && maxRecords > 0 && count >= maxRecords {
				j.Reasons = appendUnique(j.Reasons, "max_records_limit")
				limited = true
				break
			}
			q := j.Query
			if j.Plane == Runs && maxRecords > 0 {
				q.PageSize = maxRecords - count
			}
			page, err := e.Client.Fetch(ctx, j.Plane, q)
			if err != nil {
				reason := "request_failed"
				if errors.Is(err, context.Canceled) {
					reason = "cancelled"
				}
				j.Reasons = appendUnique(j.Reasons, reason)
				break
			}
			if page.Result == "permission_denied" {
				j.Reasons = appendUnique(j.Reasons, "permission_denied")
				break
			}
			fetched := e.now()
			normalize(j.Plane, q, &page, fetched, e.Client.DataSource())
			b := Batch{JobID: j.ID, Seq: j.Seq + 1, Previous: j.Query.Token, Query: q, Page: page, FetchedAt: fetched}
			trial := *j
			trial.Reasons = append([]string(nil), j.Reasons...)
			if err = apply(&trial, b); err != nil {
				return err
			}
			if err = s.Commit(b); err != nil {
				return ioError(err)
			}
			*j = trial
			if j.Plane == Runs {
				count += len(page.Items)
			}
			if err = s.SaveManifest(m); err != nil {
				return err
			}
			// Derive may append to the Jobs slice; reacquire the current pointer.
			if err = e.derive(s, m); err != nil {
				return err
			}
			j = &m.Jobs[index]
		}
		if limited && j.Plane == Runs {
			continue
		}
	}
	Summarize(m)
	if err := s.Publish(m); err != nil {
		return err
	}
	return s.SaveManifest(m)
}

type Result struct {
	DataSource   string   `json:"data_source"`
	Items        []Item   `json:"items"`
	Total        int      `json:"total"`
	Completeness string   `json:"completeness"`
	Reasons      []string `json:"reason_codes"`
	Pages        []Page   `json:"pages"`
}

func (e Engine) Collect(ctx context.Context, plane string, q Query, maxRecords int) (Result, error) {
	out := Result{DataSource: e.Client.DataSource(), Items: []Item{}, Completeness: "complete", Pages: []Page{}}
	seen := map[string]bool{}
	windows := []Window{q.Window}
	if plane == Runs {
		windows = Windows(q.Window)
	}
	for _, window := range windows {
		q.Window = window
		q.Token = ""
		for {
			if maxRecords > 0 {
				if len(out.Items) >= maxRecords {
					out.Completeness = "truncated"
					out.Reasons = appendUnique(out.Reasons, "max_records_limit")
					out.Total = len(out.Items)
					return out, nil
				}
				q.PageSize = maxRecords - len(out.Items)
			}
			p, err := e.Client.Fetch(ctx, plane, q)
			if err != nil {
				return out, err
			}
			normalize(plane, q, &p, e.now(), e.Client.DataSource())
			out.Pages = append(out.Pages, p)
			for _, i := range p.Items {
				k := Key(plane, i)
				if !seen[k] {
					seen[k] = true
					out.Items = append(out.Items, i)
				}
			}
			if p.Coverage != Guaranteed {
				out.Completeness = "partial"
				out.Reasons = appendUnique(out.Reasons, "coverage_unverified")
			}
			if p.Result != "ok" {
				out.Completeness = "partial"
				out.Reasons = appendUnique(out.Reasons, p.Result)
			}
			if plane == Buttons && (!p.Caps.CompositeCursor || (p.Caps.Attribution != ResponseField && p.Caps.Attribution != StrictLegacy)) {
				out.Completeness = "partial"
				out.Reasons = appendUnique(out.Reasons, "button_pagination_not_exactly_once")
			}
			if !p.HasMore {
				break
			}
			q.Token = p.Token
		}
	}
	out.Total = len(out.Items)
	return out, nil
}
func (e Engine) ButtonLogs(ctx context.Context, q Query, key, ref string) (Result, error) {
	d, err := e.Collect(ctx, Details, q, 0)
	if err != nil {
		return Result{}, err
	}
	if len(d.Pages) > 0 && d.Pages[0].RunStartedAt != nil {
		q.Window.Start = *d.Pages[0].RunStartedAt
	}
	out := Result{DataSource: e.Client.DataSource(), Items: []Item{}, Completeness: d.Completeness, Reasons: d.Reasons, Pages: []Page{}}
	seen := map[string]bool{}
	groups := map[string]bool{}
	matched := false
	for _, i := range d.Items {
		for _, g := range i.Groups {
			if (key != "" && g.Key != key) || (ref != "" && g.Ref != ref) {
				continue
			}
			matched = true
			candidates := []Group{g}
			if ref == "" {
				candidates = append(candidates, Group{Ref: "legacy:" + g.NodeID + ":" + g.Key, NodeID: g.NodeID, Key: g.Key, Legacy: true})
			}
			for _, group := range candidates {
				if groups[group.Ref] {
					continue
				}
				groups[group.Ref] = true
				q.Group = &group
				r, err := e.Collect(ctx, Buttons, q, 0)
				if err != nil {
					return out, err
				}
				if r.Completeness != "complete" {
					out.Completeness = "partial"
				}
				out.Pages = append(out.Pages, r.Pages...)
				for _, reason := range r.Reasons {
					out.Reasons = appendUnique(out.Reasons, reason)
				}
				for _, event := range r.Items {
					if !seen[event.EventID] {
						seen[event.EventID] = true
						out.Items = append(out.Items, event)
					}
				}
			}
		}
	}
	if !matched {
		return out, errs.NewValidationError(errs.SubtypeInvalidArgument, "button selector does not exist in run index")
	}
	out.Total = len(out.Items)
	return out, nil
}
