// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package meta is the typed model of the API metadata registry and the single
// place that parses it. The metadata is a fixed, regular vocabulary, so a plain
// typed json.Unmarshal replaces hand-rolled map[string]interface{} walking. Map
// key order is not preserved (Go maps are unordered); callers that need a
// deterministic sequence get fields/methods/resources sorted by name via the
// list accessors below.
package meta

import (
	"encoding/json"
	"sort"
	"strings"
)

// Option is one enum option of a field. Value is `any` (not string) so a
// metadata value that arrives as a JSON number — rather than the usual quoted
// string — coerces like Field.Enum / EnumOption.Value instead of failing the
// whole registry unmarshal and blanking the entire catalog. coerceLiteral
// normalizes it to the field's declared type.
type Option struct {
	Value       any    `json:"value"`
	Description string `json:"description"`
}

// Field is one parameter or body/response field. Name is the parent map key,
// populated by the list accessors (not a JSON field). ref/annotations/enumName
// exist in the metadata but are intentionally not modeled (unused downstream).
type Field struct {
	Name        string           `json:"-"`
	Type        string           `json:"type"`
	Location    string           `json:"location"` // "path" | "query"; empty for body/response
	Required    bool             `json:"required"`
	Description string           `json:"description"`
	Default     any              `json:"default"`
	Example     any              `json:"example"`
	Min         string           `json:"min"`
	Max         string           `json:"max"`
	Enum        []any            `json:"enum"`
	Options     []Option         `json:"options"`
	Properties  map[string]Field `json:"properties"`
}

// FlagName is the kebab-case CLI flag for this field (chat_id -> chat-id).
func (f Field) FlagName() string { return strings.ReplaceAll(f.Name, "_", "-") }

// Children returns the field's nested properties sorted by name.
func (f Field) Children() []Field { return fieldsOf(f.Properties, nil) }

// Method is one API operation. Name is the parent map key. Affordance is kept
// raw so this package stays free of envelope concerns.
type Method struct {
	Name           string           `json:"-"`
	ID             string           `json:"id"`
	Path           string           `json:"path"`
	HTTPMethod     string           `json:"httpMethod"`
	Description    string           `json:"description"`
	Risk           string           `json:"risk"`
	DocURL         string           `json:"docUrl"`
	Danger         bool             `json:"danger"`
	Tips           []string         `json:"tips"`
	Scopes         []string         `json:"scopes"`
	RequiredScopes []string         `json:"requiredScopes"`
	AccessTokens   []Token          `json:"accessTokens"`
	Affordance     json.RawMessage  `json:"affordance"`
	Parameters     map[string]Field `json:"parameters"`
	RequestBody    map[string]Field `json:"requestBody"`
	ResponseBody   map[string]Field `json:"responseBody"`
}

// Params are the path/query parameters, sorted by name.
func (m Method) Params() []Field {
	return fieldsOf(m.Parameters, func(f Field) bool {
		return f.Location == "path" || f.Location == "query"
	})
}

// Data are the non-file request-body fields (--data JSON), sorted by name.
func (m Method) Data() []Field {
	return fieldsOf(m.RequestBody, func(f Field) bool { return f.Type != "file" })
}

// Files are the file-typed request-body fields (--file uploads), sorted by name.
func (m Method) Files() []Field {
	return fieldsOf(m.RequestBody, func(f Field) bool { return f.Type == "file" })
}

// Response are the response-body fields, sorted by name.
func (m Method) Response() []Field { return fieldsOf(m.ResponseBody, nil) }

// fieldsOf materializes a name->field map into a name-injected slice, optionally
// filtered, sorted by name for deterministic output.
func fieldsOf(byName map[string]Field, keep func(Field) bool) []Field {
	out := make([]Field, 0, len(byName))
	for name, f := range byName {
		f.Name = name
		if keep == nil || keep(f) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Resource groups methods (and may nest sub-resources). Name is the parent key.
type Resource struct {
	Name      string              `json:"-"`
	Methods   map[string]Method   `json:"methods"`
	Resources map[string]Resource `json:"resources"`
}

// MethodList returns the resource's methods, name-injected and sorted by name.
func (r Resource) MethodList() []Method {
	out := make([]Method, 0, len(r.Methods))
	for name, m := range r.Methods {
		m.Name = name
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Method looks up one method by name with Name injected, or false if absent.
// Use this instead of indexing Methods directly so Name is never left empty.
func (r Resource) Method(name string) (Method, bool) {
	m, ok := r.Methods[name]
	if !ok {
		return Method{}, false
	}
	m.Name = name
	return m, true
}

// SubResources returns nested resources, name-injected and sorted by name.
func (r Resource) SubResources() []Resource { return resourcesOf(r.Resources) }

// Service is one API service. Name is a real JSON field (services is an array).
type Service struct {
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	ServicePath string              `json:"servicePath"`
	Resources   map[string]Resource `json:"resources"`
}

// ResourceList returns the service's top-level resources, name-injected and
// sorted by name.
func (s Service) ResourceList() []Resource { return resourcesOf(s.Resources) }

// Resource looks up one (possibly dotted) resource by name with Name injected,
// or false if absent. Use this instead of indexing Resources directly.
func (s Service) Resource(name string) (Resource, bool) {
	r, ok := s.Resources[name]
	if !ok {
		return Resource{}, false
	}
	r.Name = name
	return r, true
}

func resourcesOf(byName map[string]Resource) []Resource {
	out := make([]Resource, 0, len(byName))
	for name, r := range byName {
		r.Name = name
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Registry is the top-level metadata document.
type Registry struct {
	Services []Service `json:"services"`
	Version  string    `json:"version"`
}

// Parse decodes the metadata JSON into the typed Registry. Returns a zero
// Registry for empty input.
func Parse(data []byte) (Registry, error) {
	if len(data) == 0 {
		return Registry{}, nil
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return Registry{}, err
	}
	return reg, nil
}

// FromMap decodes a single method spec from its map form into a typed Method.
// Convenience constructor for building typed values from map literals (tests).
func FromMap(method map[string]interface{}) Method {
	b, err := json.Marshal(method)
	if err != nil {
		return Method{}
	}
	var m Method
	_ = json.Unmarshal(b, &m)
	return m
}

// ServiceFromMap decodes a service spec from its map form into a typed Service.
// Convenience constructor for building typed values from map literals (tests).
func ServiceFromMap(svc map[string]interface{}) Service {
	b, err := json.Marshal(svc)
	if err != nil {
		return Service{}
	}
	var s Service
	_ = json.Unmarshal(b, &s)
	return s
}
