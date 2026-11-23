// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// DryRun is an opaque ordered description of requests that execution may send.
type DryRun struct {
	description string
	requests    []Request
	files       []FileIntent
}

// File appends one logical file effect. It does not inspect the destination or
// create storage; conflict and final-location decisions remain live-only.
func (d *DryRun) File(intent FileIntent) *DryRun {
	if d == nil {
		return d
	}
	d.files = append(d.files, intent)
	return d
}

// NewDryRun creates a dry-run request list from shared Request values. Passing
// no requests creates an empty list to fill in with the chained methods below.
func NewDryRun(requests ...Request) *DryRun {
	return &DryRun{requests: append([]Request(nil), requests...)}
}

// Add appends a shared request description.
func (d *DryRun) Add(request Request) *DryRun {
	if d == nil {
		return d
	}
	d.requests = append(d.requests, request)
	return d
}

// GET appends a GET request.
func (d *DryRun) GET(apiPath string) *DryRun { return d.Add(GET(apiPath)) }

// POST appends a POST request.
func (d *DryRun) POST(apiPath string) *DryRun { return d.Add(POST(apiPath)) }

// PUT appends a PUT request.
func (d *DryRun) PUT(apiPath string) *DryRun { return d.Add(PUT(apiPath)) }

// PATCH appends a PATCH request.
func (d *DryRun) PATCH(apiPath string) *DryRun { return d.Add(PATCH(apiPath)) }

// DELETE appends a DELETE request.
func (d *DryRun) DELETE(apiPath string) *DryRun { return d.Add(DELETE(apiPath)) }

// Set adds a query parameter to the most recently appended request.
func (d *DryRun) Set(name string, value any) *DryRun {
	if d == nil || len(d.requests) == 0 {
		return d
	}
	d.requests[len(d.requests)-1] = d.requests[len(d.requests)-1].Set(name, value)
	return d
}

// Params replaces query parameters on the most recently appended request.
func (d *DryRun) Params(params map[string]any) *DryRun {
	if d == nil || len(d.requests) == 0 {
		return d
	}
	d.requests[len(d.requests)-1] = d.requests[len(d.requests)-1].Params(params)
	return d
}

// Body sets the body on the most recently appended request.
func (d *DryRun) Body(body any) *DryRun {
	if d == nil || len(d.requests) == 0 {
		return d
	}
	d.requests[len(d.requests)-1] = d.requests[len(d.requests)-1].Body(body)
	return d
}

// Desc sets a call description, or the top-level description before any call exists.
func (d *DryRun) Desc(description string) *DryRun {
	if d == nil {
		return d
	}
	if len(d.requests) == 0 {
		d.description = description
		return d
	}
	d.requests[len(d.requests)-1] = d.requests[len(d.requests)-1].Desc(description)
	return d
}

// DryRunView is a copied host and test projection of DryRun.
type DryRunView struct {
	Description string
	Requests    []RequestView
	Files       []FileIntent
}

// InspectDryRun returns a copied dry-run projection for host adapters and tests.
func InspectDryRun(dryRun *DryRun) DryRunView {
	if dryRun == nil {
		return DryRunView{}
	}
	view := DryRunView{
		Description: dryRun.description,
		Requests:    make([]RequestView, len(dryRun.requests)),
		Files:       append([]FileIntent(nil), dryRun.files...),
	}
	for index, request := range dryRun.requests {
		view.Requests[index] = InspectRequest(request)
	}
	return view
}
