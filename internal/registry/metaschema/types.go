// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package metaschema defines the typed shape of the command-spec registry
// (meta_data.json). The embedded baseline is emitted as static Go data in
// package metastatic (no runtime JSON parse, no startup allocation); the remote
// overlay is decoded into these same types at runtime.
//
// All container fields are slices (never maps): a package-level slice literal is
// laid out in the binary's data section and costs zero heap allocation at
// startup, whereas a map literal builds an hmap at init time. Map keys from the
// JSON (resource/method/field names) are preserved in the Name field.
package metaschema

// Registry is the top level of meta_data.json: {version, services:[...]}.
type Registry struct {
	Version  string
	Services []Service
}

// Service is one API domain (e.g. "im", "calendar").
type Service struct {
	Name        string
	Version     string
	Title       string
	Description string
	ServicePath string
	Resources   []Resource // JSON "resources" map, keyed by Resource.Name
}

// Resource groups methods under a service (e.g. "messages").
type Resource struct {
	Name    string
	Methods []Method // JSON "methods" map, keyed by Method.Name
}

// Method is a single API call.
type Method struct {
	Name           string // JSON map key
	ID             string
	Path           string
	HTTPMethod     string
	Description    string
	Risk           string
	DocURL         string
	Danger         bool
	Scopes         []string
	AccessTokens   []string
	ParameterOrder []string
	RequiredScopes []string
	Parameters     []Field     // JSON "parameters" map, keyed by Field.Name
	RequestBody    []Field     // JSON "requestBody" map
	ResponseBody   []Field     // JSON "responseBody" map
	Affordance     *Affordance // optional AI-facing usage overlay; nil on most methods
}

// Field is one parameter / request-body / response-body entry. Nested object
// fields recurse via Properties.
type Field struct {
	Name        string // JSON map key
	Type        string
	Location    string
	Description string
	Default     string
	Example     string
	EnumName    string
	Min         string
	Max         string
	Ref         string
	Required    bool
	Options     []Option
	Enum        []string
	Annotations []string
	Properties  []Field
}

// Option is one allowed value for a field with an enum-like option list.
type Option struct {
	Value       string
	Description string
}

// Affordance is the optional AI-facing usage overlay for a method, surfaced in
// the schema envelope as _meta.affordance. Absent (nil) on most methods; it is
// authored upstream in registry-config.yaml and merged into meta_data.json.
type Affordance struct {
	UseWhen       []string
	DoNotUseWhen  []string
	Prerequisites []string
	Examples      []AffordanceExample
	Related       []string
}

// AffordanceExample is one ready-to-run example: a one-line description plus a
// complete lark-cli command string.
type AffordanceExample struct {
	Description string
	Command     string
}
