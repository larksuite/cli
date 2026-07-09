// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package apicatalog is the single navigation Module over the API metadata. It
// owns every "which services/resources/methods exist and how does a path
// resolve" question that was previously duplicated across cmd/schema,
// cmd/service, internal/schema and internal/registry. It depends only on
// internal/meta; registry is the source Adapter (EmbeddedCatalog/RuntimeCatalog),
// so apicatalog never imports registry.
package apicatalog

import (
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/meta"
)

// Source records whether a catalog includes the remote overlay. It is carried
// so callers (and tests) can assert determinism instead of guessing.
type Source string

const (
	SourceEmbedded Source = "embedded" // compiled-in metadata only; deterministic
	SourceRuntime  Source = "runtime"  // embedded + remote overlay
)

// MethodFilter optionally drops methods (e.g. by identity in strict mode).
// A nil filter includes everything.
type MethodFilter func(meta.Method) bool

// Catalog is a navigation view over services with a name index. It owns its
// ordering — New sorts by name — so WalkMethods/Resolve/Complete are
// deterministic regardless of how the source adapter ordered its input.
type Catalog struct {
	source   Source
	services []meta.Service
	byName   map[string]meta.Service
}

// New builds a Catalog over the given services, owning its navigation order:
// the slice is copied and sorted by name so callers may pass any order and the
// ordering contract is not delegated to the adapter. The copy is shallow —
// meta.Service values share their Resources maps, which are treated as
// read-only.
func New(source Source, services []meta.Service) Catalog {
	sorted := append([]meta.Service(nil), services...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	byName := make(map[string]meta.Service, len(sorted))
	for _, s := range sorted {
		byName[s.Name] = s
	}
	return Catalog{source: source, services: sorted, byName: byName}
}

// Source reports embedded vs runtime.
func (c Catalog) Source() Source { return c.source }

// Services returns the services in name order. Treat the result as read-only:
// it is the Catalog's own ordered slice and its element Resources maps are
// shared.
func (c Catalog) Services() []meta.Service { return c.services }

// Service looks up one service by name.
func (c Catalog) Service(name string) (meta.Service, bool) {
	s, ok := c.byName[name]
	return s, ok
}

// Resolve maps a path (already split into segments) to a Target. An empty path
// is TargetAll. Failures return a *ResolveError carrying the available
// candidates so the command layer can render a hint.
func (c Catalog) Resolve(parts []string) (Target, error) {
	if len(parts) == 0 {
		return Target{Kind: TargetAll}, nil
	}
	svc, ok := c.byName[parts[0]]
	if !ok {
		return Target{}, &ResolveError{Kind: ErrService, Subject: parts[0], Candidates: c.serviceNames()}
	}
	if len(parts) == 1 {
		return Target{Kind: TargetService, Service: svc}, nil
	}
	res, path, remaining, ok := findResource(svc, parts[1:])
	if !ok {
		return Target{}, &ResolveError{
			Kind:       ErrResource,
			Subject:    svc.Name + "." + strings.Join(parts[1:], "."),
			Candidates: resourceNames(svc),
		}
	}
	resPath := strings.Join(path, ".")
	if len(remaining) == 0 {
		return Target{Kind: TargetResource, Service: svc, Resource: &ResourceRef{Service: svc, Resource: res, Path: path}}, nil
	}
	methodName := remaining[0]
	m, ok := res.Method(methodName)
	if !ok {
		return Target{}, &ResolveError{
			Kind:       ErrMethod,
			Subject:    svc.Name + "." + resPath + "." + methodName,
			Candidates: methodNames(res),
		}
	}
	if len(remaining) > 1 {
		// Method exists but trailing segments don't resolve — reject so a typo
		// doesn't silently return this method's schema.
		return Target{}, &ResolveError{
			Kind:     ErrPath,
			Subject:  svc.Name + "." + resPath + "." + strings.Join(remaining, "."),
			Method:   methodName,
			Trailing: strings.Join(remaining[1:], "."),
		}
	}
	return Target{Kind: TargetMethod, Service: svc, Method: &MethodRef{Service: svc, Resource: res, ResourcePath: path, Method: m}}, nil
}

// MethodRefs returns the method refs selected by a resolved Target, filtered:
// TargetAll -> every method, TargetService / TargetResource -> that subtree,
// TargetMethod -> the single method if it passes the filter (else empty). It
// unifies WalkMethods/ServiceMethods/ResourceMethods so the command layer maps a
// Target to refs in one call instead of re-deciding the walker per Kind.
func (c Catalog) MethodRefs(target Target, filter MethodFilter) []MethodRef {
	switch target.Kind {
	case TargetService:
		return ServiceMethods(target.Service, filter)
	case TargetResource:
		return ResourceMethods(*target.Resource, filter)
	case TargetMethod:
		if filter != nil && !filter(target.Method.Method) {
			return nil
		}
		return []MethodRef{*target.Method}
	case TargetAll:
		return c.WalkMethods(filter)
	default:
		// Unknown / zero-value Kind: return nothing rather than silently
		// dumping every method (the safe direction for an invalid Target).
		return nil
	}
}

// WalkMethods returns one MethodRef per method across all services (optionally
// filtered), recursing nested resources, in a deterministic order: services by
// name, resources by name, methods by name.
func (c Catalog) WalkMethods(filter MethodFilter) []MethodRef {
	var out []MethodRef
	for _, svc := range c.services {
		out = append(out, ServiceMethods(svc, filter)...)
	}
	return out
}

// ServiceMethods returns the method refs of one service (filtered), recursing
// nested resources, in deterministic resource/method name order.
func ServiceMethods(svc meta.Service, filter MethodFilter) []MethodRef {
	var out []MethodRef
	walkResources(svc, svc.ResourceList(), nil, filter, &out)
	return out
}

// ResourceMethods returns the method refs under one resource (filtered), using
// the resource's resolved path as the base and recursing nested resources.
func ResourceMethods(r ResourceRef, filter MethodFilter) []MethodRef {
	var out []MethodRef
	for _, m := range r.Resource.MethodList() {
		if filter == nil || filter(m) {
			out = append(out, MethodRef{Service: r.Service, Resource: r.Resource, ResourcePath: r.Path, Method: m})
		}
	}
	walkResources(r.Service, r.Resource.SubResources(), r.Path, filter, &out)
	return out
}

func walkResources(svc meta.Service, resources []meta.Resource, parentPath []string, filter MethodFilter, out *[]MethodRef) {
	for _, res := range resources {
		path := append(append([]string(nil), parentPath...), res.Name)
		for _, m := range res.MethodList() {
			if filter == nil || filter(m) {
				*out = append(*out, MethodRef{Service: svc, Resource: res, ResourcePath: path, Method: m})
			}
		}
		walkResources(svc, res.SubResources(), path, filter, out)
	}
}

// Complete returns shell-completion candidates for the schema path argument,
// supporting both the legacy single dotted arg ("im.reac") and the
// space-separated form ("im reactions"). noSpace mirrors cobra's
// ShellCompDirectiveNoSpace (so "service." / "service.resource." stay open for
// the next segment). Filtering uses the caller's MethodFilter so strict-mode
// unavailable methods are hidden.
func (c Catalog) Complete(args []string, toComplete string, filter MethodFilter) (completions []string, noSpace bool) {
	// Case 1: legacy single dotted arg — no resolved args yet.
	if len(args) == 0 {
		parts := strings.Split(toComplete, ".")
		if len(parts) <= 1 {
			for _, name := range c.serviceNames() {
				if strings.HasPrefix(name, toComplete) {
					completions = append(completions, name+".")
				}
			}
			return completions, true
		}
		svc, ok := c.byName[parts[0]]
		if !ok {
			return nil, false
		}
		completions = c.completeDotted(svc, strings.Join(parts[1:], "."), filter)
		allTrailingDot := len(completions) > 0
		for _, comp := range completions {
			if !strings.HasSuffix(comp, ".") {
				allTrailingDot = false
				break
			}
		}
		return completions, allTrailingDot
	}

	// Case 2: space-separated form — args holds resolved segments.
	svc, ok := c.byName[args[0]]
	if !ok {
		return nil, false
	}
	resource, _, _, ok := findResource(svc, args[1:])
	if !ok {
		// No resource matched yet — suggest top-level resources reachable in the
		// current identity mode.
		return completeChildren(svc.ResourceList(), nil, toComplete, filter), false
	}
	// Positioned in a resource — offer its methods and its sub-resources, so the
	// next segment can drill deeper, symmetric to findResource's descent.
	return completeChildren(resource.SubResources(), resource.MethodList(), toComplete, filter), false
}

// completeDotted suggests dotted completions for the text after the service
// segment. It descends fully-typed "resource." segments (longest match per
// level, so flat dotted keys like "chat.members" and genuinely nested resources
// both resolve), then offers the reachable sub-resources (as "…name.") and the
// methods (as "…name") of the level it lands in whose names extend the trailing
// partial token. This descent is symmetric to findResource, so completion can
// reach every method Resolve can.
func (c Catalog) completeDotted(svc meta.Service, afterService string, filter MethodFilter) []string {
	subs := svc.ResourceList()
	base := svc.Name
	rest := afterService
	var here *meta.Resource // resource we're positioned in; nil at the service root
	for {
		matched, n, ok := longestResourceFollowedByDot(subs, rest)
		if !ok {
			break
		}
		base += "." + matched.Name
		rest = rest[n:]
		r := matched
		here = &r
		subs = matched.SubResources()
	}

	var out []string
	for _, sub := range subs {
		if strings.HasPrefix(sub.Name, rest) && resourceReachable(sub, filter) {
			out = append(out, base+"."+sub.Name+".")
		}
	}
	if here != nil {
		for _, m := range here.MethodList() {
			if (filter == nil || filter(m)) && strings.HasPrefix(m.Name, rest) {
				out = append(out, base+"."+m.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// completeChildren returns the sorted next-segment candidates at one level: the
// (filtered) methods and the reachable sub-resources whose names extend prefix.
// Methods are terminal; sub-resources are bare names the caller drills into on
// the next segment.
func completeChildren(subResources []meta.Resource, methods []meta.Method, prefix string, filter MethodFilter) []string {
	var out []string
	for _, m := range methods {
		if (filter == nil || filter(m)) && strings.HasPrefix(m.Name, prefix) {
			out = append(out, m.Name)
		}
	}
	for _, sub := range subResources {
		if strings.HasPrefix(sub.Name, prefix) && resourceReachable(sub, filter) {
			out = append(out, sub.Name)
		}
	}
	sort.Strings(out)
	return out
}

// longestResourceFollowedByDot finds the longest resource in resources whose
// name is a fully-typed segment of text (text begins with "name."), returning
// it, the byte length consumed (incl. the dot), and whether one matched.
func longestResourceFollowedByDot(resources []meta.Resource, text string) (meta.Resource, int, bool) {
	best := meta.Resource{}
	bestLen := -1
	for _, r := range resources {
		if len(r.Name) > bestLen && strings.HasPrefix(text, r.Name+".") {
			best = r
			bestLen = len(r.Name)
		}
	}
	if bestLen < 0 {
		return meta.Resource{}, 0, false
	}
	return best, len(best.Name) + 1, true
}

// findResource resolves a resource path against a service, descending nested
// resources. At each level it consumes the longest leading run of parts that
// names a resource at that level, so both flat dotted keys ("chat.members")
// and genuinely nested resources ("spaces" > "items") resolve. This descent is
// symmetric to walkResources, which guarantees every path WalkMethods emits
// resolves back (the round-trip contract). Returns the deepest matched resource
// (Name injected), its path segments, the unconsumed remainder, and whether
// anything matched.
//
// Descent is greedy and resource-first: the one ambiguous case is a resource
// that has BOTH a method and a sub-resource of the same name — the sub-resource
// wins and shadows the method, so Resolve can never reach that method. Real
// metadata never collides the two, so this is theoretical.
func findResource(svc meta.Service, parts []string) (res meta.Resource, path []string, remaining []string, ok bool) {
	level := svc.Resources
	remaining = parts
	for len(remaining) > 0 {
		matched, name, n := longestResourcePrefix(level, remaining)
		if n == 0 {
			break
		}
		matched.Name = name
		res = matched
		path = append(path, name)
		remaining = remaining[n:]
		level = matched.Resources
		ok = true
	}
	return res, path, remaining, ok
}

// longestResourcePrefix finds the longest leading run of segs (joined by ".")
// that names a resource in level, returning the resource, its dotted name, and
// the number of segments consumed (0 if none match). Longest-first lets a flat
// dotted key win over its single leading segment when present.
func longestResourcePrefix(level map[string]meta.Resource, segs []string) (meta.Resource, string, int) {
	for i := len(segs); i >= 1; i-- {
		name := strings.Join(segs[:i], ".")
		if r, ok := level[name]; ok {
			return r, name, i
		}
	}
	return meta.Resource{}, "", 0
}

// resourceReachable reports whether a resource exposes a method reachable under
// the filter — directly or in any nested sub-resource (a nil filter accepts any
// method). A resource whose methods are all filtered out but which contains a
// reachable nested method is still offerable, so completion can drill into it.
func resourceReachable(res meta.Resource, filter MethodFilter) bool {
	for _, m := range res.MethodList() {
		if filter == nil || filter(m) {
			return true
		}
	}
	for _, sub := range res.SubResources() {
		if resourceReachable(sub, filter) {
			return true
		}
	}
	return false
}

func (c Catalog) serviceNames() []string {
	names := make([]string, len(c.services))
	for i, s := range c.services {
		names[i] = s.Name
	}
	return names // c.services is already name-sorted
}

func resourceNames(svc meta.Service) []string { return sortedKeys(svc.Resources) }
func methodNames(res meta.Resource) []string  { return sortedKeys(res.Methods) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
