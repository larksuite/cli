// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/larksuite/cli/internal/registry/metaschema"
	"github.com/larksuite/cli/internal/registry/metastatic"
	"github.com/larksuite/cli/internal/vfs"
)

// This file is the typed registry layer for the static-meta migration.
//
//   - The embedded baseline is metastatic.Registry: static Go data laid out in
//     the binary at compile time (zero startup cost). It is empty on a fresh
//     checkout (stub.go) until the generated meta_data_gen.go is produced by
//     `make fetch_meta`; no build tag is involved.
//   - The remote overlay (~/.lark-cli/cache/remote_meta.json) is still fetched
//     and refreshed at runtime, decoded into the same typed shape, and merged
//     over the baseline as per-service overrides.
//
// Startup (command-tree build) reads these typed structs directly. Execution-
// path consumers that still expect map[string]interface{} go through
// ServiceToMap, which rebuilds one service's map lazily, on demand — never the
// whole spec at startup.

var (
	typedMu         sync.RWMutex
	remoteOverrides map[string]metaschema.Service // service name -> remote override
	typedNamesCache []string
)

// resetTyped clears the typed overlay state (test/teardown helper).
func resetTyped() {
	typedMu.Lock()
	defer typedMu.Unlock()
	remoteOverrides = nil
	typedNamesCache = nil
}

// baselineServices returns the embedded baseline service specs: the static
// compile-time data in metastatic.Registry (zero parse, zero alloc). It is
// empty only on a fresh checkout where meta_data_gen.go has not been generated
// yet (see stub.go).
var (
	baselineOnce sync.Once
	baselineSvcs []metaschema.Service
	baselineVer  string
)

func loadBaseline() {
	baselineOnce.Do(func() {
		baselineSvcs = metastatic.Registry.Services
		baselineVer = metastatic.Registry.Version
	})
}

func baselineServices() []metaschema.Service {
	loadBaseline()
	return baselineSvcs
}

func baselineVersion() string {
	loadBaseline()
	return baselineVer
}

// baselineServiceByName returns the embedded baseline service spec by name.
func baselineServiceByName(name string) (metaschema.Service, bool) {
	svcs := baselineServices()
	for i := range svcs {
		if svcs[i].Name == name {
			return svcs[i], true
		}
	}
	return metaschema.Service{}, false
}

// typedServiceByName returns the effective typed spec for a service: the remote
// override if present, otherwise the static baseline.
func typedServiceByName(name string) (metaschema.Service, bool) {
	typedMu.RLock()
	if s, ok := remoteOverrides[name]; ok {
		typedMu.RUnlock()
		return s, true
	}
	typedMu.RUnlock()
	return baselineServiceByName(name)
}

// typedServiceNames returns all effective service names (baseline + remote
// additions), sorted. Cached until the overlay changes.
func typedServiceNames() []string {
	typedMu.RLock()
	if typedNamesCache != nil {
		out := typedNamesCache
		typedMu.RUnlock()
		return out
	}
	typedMu.RUnlock()

	seen := make(map[string]bool)
	for _, s := range baselineServices() {
		seen[s.Name] = true
	}
	typedMu.RLock()
	for name := range remoteOverrides {
		seen[name] = true
	}
	typedMu.RUnlock()

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)

	typedMu.Lock()
	typedNamesCache = names
	typedMu.Unlock()
	return names
}

// setRemoteOverrides installs the parsed remote overlay (called from Init).
func setRemoteOverrides(svcs []metaschema.Service) {
	typedMu.Lock()
	defer typedMu.Unlock()
	if remoteOverrides == nil {
		remoteOverrides = make(map[string]metaschema.Service, len(svcs))
	}
	for _, s := range svcs {
		remoteOverrides[s.Name] = s
	}
	typedNamesCache = nil
}

// TypedService returns the effective typed spec for a service (remote override
// or static baseline). Public accessor for the command-tree builder.
func TypedService(name string) (metaschema.Service, bool) {
	Init()
	return typedServiceByName(name)
}

// TypedServices returns all effective service specs, sorted by name. Reading
// these builds nothing on the heap (static data); the remote overlay, if any,
// was allocated once at Init.
func TypedServices() []metaschema.Service {
	Init()
	names := typedServiceNames()
	out := make([]metaschema.Service, 0, len(names))
	for _, n := range names {
		if s, ok := typedServiceByName(n); ok {
			out = append(out, s)
		}
	}
	return out
}

// hasTypedData reports whether any typed spec is available (static baseline or
// remote overlay). False only when the static registry has not been generated
// (fresh checkout) and there is no cache.
func hasTypedData() bool {
	if len(baselineServices()) > 0 {
		return true
	}
	typedMu.RLock()
	defer typedMu.RUnlock()
	return len(remoteOverrides) > 0
}

// loadCachedTyped reads the on-disk remote cache, decodes it into the typed
// shape, and installs it as the remote overlay (typed replacement for the old
// map-based loadCachedMerged + overlay).
func loadCachedTyped() error {
	data, err := vfs.ReadFile(cachePath())
	if err != nil {
		return err
	}
	var reg wireRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		// Cache corrupted — remove it so the next run triggers a fresh fetch.
		_ = vfs.Remove(cachePath())
		_ = vfs.Remove(cacheMetaPath())
		return err
	}
	svcs := make([]metaschema.Service, 0, len(reg.Services))
	for _, ws := range reg.Services {
		svcs = append(svcs, wireToService(ws))
	}
	setRemoteOverrides(svcs)
	return nil
}

// --- typed -> map[string]interface{} shim (lazy, per service, execution-path) ---

func strList(ss []string) []interface{} {
	if len(ss) == 0 {
		return nil
	}
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func fieldToMap(f metaschema.Field) map[string]interface{} {
	m := map[string]interface{}{}
	put := func(k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	put("type", f.Type)
	put("location", f.Location)
	put("description", f.Description)
	put("default", f.Default)
	put("example", f.Example)
	put("enumName", f.EnumName)
	put("min", f.Min)
	put("max", f.Max)
	put("ref", f.Ref)
	if f.Required {
		m["required"] = true
	}
	if v := strList(f.Enum); v != nil {
		m["enum"] = v
	}
	if v := strList(f.Annotations); v != nil {
		m["annotations"] = v
	}
	if len(f.Options) > 0 {
		opts := make([]interface{}, len(f.Options))
		for i, o := range f.Options {
			opts[i] = map[string]interface{}{"value": o.Value, "description": o.Description}
		}
		m["options"] = opts
	}
	if len(f.Properties) > 0 {
		m["properties"] = fieldsToMap(f.Properties)
	}
	return m
}

func fieldsToMap(fs []metaschema.Field) map[string]interface{} {
	if len(fs) == 0 {
		return nil
	}
	m := make(map[string]interface{}, len(fs))
	for _, f := range fs {
		m[f.Name] = fieldToMap(f)
	}
	return m
}

// affordanceToMap rebuilds the JSON-shaped affordance object (snake_case keys)
// so the schema assembler's parseAffordance(method["affordance"]) keeps working
// through the typed registry. Returns nil when the overlay carries nothing.
func affordanceToMap(a *metaschema.Affordance) map[string]interface{} {
	m := map[string]interface{}{}
	if v := strList(a.UseWhen); v != nil {
		m["use_when"] = v
	}
	if v := strList(a.DoNotUseWhen); v != nil {
		m["do_not_use_when"] = v
	}
	if v := strList(a.Prerequisites); v != nil {
		m["prerequisites"] = v
	}
	if len(a.Examples) > 0 {
		ex := make([]interface{}, len(a.Examples))
		for i, e := range a.Examples {
			ex[i] = map[string]interface{}{"description": e.Description, "command": e.Command}
		}
		m["examples"] = ex
	}
	if v := strList(a.Related); v != nil {
		m["related"] = v
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func MethodToMap(mth metaschema.Method) map[string]interface{} {
	m := map[string]interface{}{
		"id":          mth.ID,
		"path":        mth.Path,
		"httpMethod":  mth.HTTPMethod,
		"description": mth.Description,
	}
	if mth.Risk != "" {
		m["risk"] = mth.Risk
	}
	if mth.DocURL != "" {
		m["docUrl"] = mth.DocURL
	}
	if mth.Danger {
		m["danger"] = true
	}
	if v := strList(mth.Scopes); v != nil {
		m["scopes"] = v
	}
	if v := strList(mth.AccessTokens); v != nil {
		m["accessTokens"] = v
	}
	if v := strList(mth.ParameterOrder); v != nil {
		m["parameterOrder"] = v
	}
	if v := strList(mth.RequiredScopes); v != nil {
		m["requiredScopes"] = v
	}
	if v := fieldsToMap(mth.Parameters); v != nil {
		m["parameters"] = v
	}
	if v := fieldsToMap(mth.RequestBody); v != nil {
		m["requestBody"] = v
	}
	if v := fieldsToMap(mth.ResponseBody); v != nil {
		m["responseBody"] = v
	}
	if mth.Affordance != nil {
		if am := affordanceToMap(mth.Affordance); am != nil {
			m["affordance"] = am
		}
	}
	return m
}

// ServiceToMap rebuilds the JSON-shaped map[string]interface{} for one service,
// so execution-path consumers (and method RunE) keep working unchanged.
func ServiceToMap(s metaschema.Service) map[string]interface{} {
	resources := make(map[string]interface{}, len(s.Resources))
	for _, r := range s.Resources {
		methods := make(map[string]interface{}, len(r.Methods))
		for _, mth := range r.Methods {
			methods[mth.Name] = MethodToMap(mth)
		}
		resources[r.Name] = map[string]interface{}{"methods": methods}
	}
	return map[string]interface{}{
		"name":        s.Name,
		"version":     s.Version,
		"title":       s.Title,
		"description": s.Description,
		"servicePath": s.ServicePath,
		"resources":   resources,
	}
}

// --- map[string]interface{} -> typed (for the map-based wrappers still used by
// tests; production builds from typed directly) ---

func ifaceStrs(v interface{}) []string {
	raw, _ := v.([]interface{})
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sortedMapKeys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func mapToField(name string, m map[string]interface{}) metaschema.Field {
	f := metaschema.Field{
		Name: name, Type: GetStrFromMap(m, "type"), Location: GetStrFromMap(m, "location"),
		Description: GetStrFromMap(m, "description"), Default: GetStrFromMap(m, "default"),
		Example: GetStrFromMap(m, "example"), EnumName: GetStrFromMap(m, "enumName"),
		Min: GetStrFromMap(m, "min"), Max: GetStrFromMap(m, "max"), Ref: GetStrFromMap(m, "ref"),
		Enum: ifaceStrs(m["enum"]), Annotations: ifaceStrs(m["annotations"]),
	}
	if b, ok := m["required"].(bool); ok {
		f.Required = b
	}
	if opts, ok := m["options"].([]interface{}); ok {
		for _, o := range opts {
			om, _ := o.(map[string]interface{})
			f.Options = append(f.Options, metaschema.Option{Value: GetStrFromMap(om, "value"), Description: GetStrFromMap(om, "description")})
		}
	}
	f.Properties = mapToFields(m["properties"])
	return f
}

func mapToFields(v interface{}) []metaschema.Field {
	fm, _ := v.(map[string]interface{})
	if len(fm) == 0 {
		return nil
	}
	out := make([]metaschema.Field, 0, len(fm))
	for _, k := range sortedMapKeys(fm) {
		em, _ := fm[k].(map[string]interface{})
		out = append(out, mapToField(k, em))
	}
	return out
}

func MapToMethod(name string, m map[string]interface{}) metaschema.Method {
	return metaschema.Method{
		Name: name, ID: GetStrFromMap(m, "id"), Path: GetStrFromMap(m, "path"),
		HTTPMethod: GetStrFromMap(m, "httpMethod"), Description: GetStrFromMap(m, "description"),
		Risk: GetStrFromMap(m, "risk"), DocURL: GetStrFromMap(m, "docUrl"),
		Danger:         boolFromMap(m, "danger"),
		Scopes:         ifaceStrs(m["scopes"]),
		AccessTokens:   ifaceStrs(m["accessTokens"]),
		ParameterOrder: ifaceStrs(m["parameterOrder"]),
		RequiredScopes: ifaceStrs(m["requiredScopes"]),
		Parameters:     mapToFields(m["parameters"]),
		RequestBody:    mapToFields(m["requestBody"]),
		ResponseBody:   mapToFields(m["responseBody"]),
	}
}

func boolFromMap(m map[string]interface{}, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func MapToResources(v interface{}) []metaschema.Resource {
	rm, _ := v.(map[string]interface{})
	if len(rm) == 0 {
		return nil
	}
	out := make([]metaschema.Resource, 0, len(rm))
	for _, rk := range sortedMapKeys(rm) {
		res, _ := rm[rk].(map[string]interface{})
		mm, _ := res["methods"].(map[string]interface{})
		methods := make([]metaschema.Method, 0, len(mm))
		for _, mk := range sortedMapKeys(mm) {
			methodMap, _ := mm[mk].(map[string]interface{})
			methods = append(methods, MapToMethod(mk, methodMap))
		}
		out = append(out, metaschema.Resource{Name: rk, Methods: methods})
	}
	return out
}

// MapToService converts a JSON-shaped service spec (with embedded "resources")
// into the typed form.
func MapToService(spec map[string]interface{}) metaschema.Service {
	return metaschema.Service{
		Name: GetStrFromMap(spec, "name"), Version: GetStrFromMap(spec, "version"),
		Title: GetStrFromMap(spec, "title"), Description: GetStrFromMap(spec, "description"),
		ServicePath: GetStrFromMap(spec, "servicePath"), Resources: MapToResources(spec["resources"]),
	}
}

// --- remote JSON (wire) -> typed ---

type wireRegistry struct {
	Version  string        `json:"version"`
	Services []wireService `json:"services"`
}

type wireService struct {
	Name        string                  `json:"name"`
	Version     string                  `json:"version"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	ServicePath string                  `json:"servicePath"`
	Resources   map[string]wireResource `json:"resources"`
}

type wireResource struct {
	Methods map[string]wireMethod `json:"methods"`
}

type wireMethod struct {
	ID             string               `json:"id"`
	Path           string               `json:"path"`
	HTTPMethod     string               `json:"httpMethod"`
	Description    string               `json:"description"`
	Risk           string               `json:"risk"`
	DocURL         string               `json:"docUrl"`
	Danger         bool                 `json:"danger"`
	Scopes         []string             `json:"scopes"`
	AccessTokens   []string             `json:"accessTokens"`
	ParameterOrder []string             `json:"parameterOrder"`
	RequiredScopes []string             `json:"requiredScopes"`
	Parameters     map[string]wireField `json:"parameters"`
	RequestBody    map[string]wireField `json:"requestBody"`
	ResponseBody   map[string]wireField `json:"responseBody"`
}

type wireField struct {
	Type        string               `json:"type"`
	Location    string               `json:"location"`
	Description string               `json:"description"`
	Default     string               `json:"default"`
	Example     string               `json:"example"`
	EnumName    string               `json:"enumName"`
	Min         string               `json:"min"`
	Max         string               `json:"max"`
	Ref         string               `json:"ref"`
	Required    bool                 `json:"required"`
	Options     []metaschema.Option  `json:"options"`
	Enum        []string             `json:"enum"`
	Annotations []string             `json:"annotations"`
	Properties  map[string]wireField `json:"properties"`
}

func sortedFieldKeys(m map[string]wireField) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func wireFields(m map[string]wireField) []metaschema.Field {
	if len(m) == 0 {
		return nil
	}
	out := make([]metaschema.Field, 0, len(m))
	for _, name := range sortedFieldKeys(m) {
		wf := m[name]
		out = append(out, metaschema.Field{
			Name: name, Type: wf.Type, Location: wf.Location, Description: wf.Description,
			Default: wf.Default, Example: wf.Example, EnumName: wf.EnumName,
			Min: wf.Min, Max: wf.Max, Ref: wf.Ref, Required: wf.Required,
			Options: wf.Options, Enum: wf.Enum, Annotations: wf.Annotations,
			Properties: wireFields(wf.Properties),
		})
	}
	return out
}

func wireToService(ws wireService) metaschema.Service {
	resKeys := make([]string, 0, len(ws.Resources))
	for k := range ws.Resources {
		resKeys = append(resKeys, k)
	}
	sort.Strings(resKeys)
	resources := make([]metaschema.Resource, 0, len(resKeys))
	for _, rk := range resKeys {
		wr := ws.Resources[rk]
		methKeys := make([]string, 0, len(wr.Methods))
		for k := range wr.Methods {
			methKeys = append(methKeys, k)
		}
		sort.Strings(methKeys)
		methods := make([]metaschema.Method, 0, len(methKeys))
		for _, mk := range methKeys {
			wm := wr.Methods[mk]
			methods = append(methods, metaschema.Method{
				Name: mk, ID: wm.ID, Path: wm.Path, HTTPMethod: wm.HTTPMethod,
				Description: wm.Description, Risk: wm.Risk, DocURL: wm.DocURL, Danger: wm.Danger,
				Scopes: wm.Scopes, AccessTokens: wm.AccessTokens,
				ParameterOrder: wm.ParameterOrder, RequiredScopes: wm.RequiredScopes,
				Parameters: wireFields(wm.Parameters), RequestBody: wireFields(wm.RequestBody),
				ResponseBody: wireFields(wm.ResponseBody),
			})
		}
		resources = append(resources, metaschema.Resource{Name: rk, Methods: methods})
	}
	return metaschema.Service{
		Name: ws.Name, Version: ws.Version, Title: ws.Title,
		Description: ws.Description, ServicePath: ws.ServicePath, Resources: resources,
	}
}
