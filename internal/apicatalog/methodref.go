// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog

import (
	"strings"

	"github.com/larksuite/cli/internal/meta"
)

// TargetKind classifies what a schema/command path resolves to.
type TargetKind string

const (
	TargetAll      TargetKind = "all"      // empty path: every method
	TargetService  TargetKind = "service"  // <service>
	TargetResource TargetKind = "resource" // <service> <resource...>
	TargetMethod   TargetKind = "method"   // <service> <resource...> <method>
)

// Target is the result of Catalog.Resolve. Resource and Method are populated
// only for TargetResource and TargetMethod respectively.
type Target struct {
	Kind     TargetKind
	Service  meta.Service
	Resource *ResourceRef
	Method   *MethodRef
}

// ResourceRef identifies one resource within a service. Path holds the resource
// path segments (one element for the common flat dotted resource like
// "chat.members"; multiple for genuinely nested resources).
type ResourceRef struct {
	Service  meta.Service
	Resource meta.Resource
	Path     []string
}

// MethodRef identifies one method, carrying the full navigation context so the
// command path and schema path can be derived without re-walking the catalog.
type MethodRef struct {
	Service      meta.Service
	Resource     meta.Resource
	ResourcePath []string
	Method       meta.Method
}

// SchemaPath is the dotted "service.resource" identifier.
func (r ResourceRef) SchemaPath() string {
	return r.Service.Name + "." + strings.Join(r.Path, ".")
}

// ServiceName returns the owning service name.
func (r MethodRef) ServiceName() string { return r.Service.Name }

// ResourceName is the dotted resource path, e.g. "chat.members".
func (r MethodRef) ResourceName() string { return strings.Join(r.ResourcePath, ".") }

// MethodName returns the method's own name.
func (r MethodRef) MethodName() string { return r.Method.Name }

// SchemaPath is the dotted "service.resource.method" identifier, e.g.
// "im.chat.members.create".
func (r MethodRef) SchemaPath() string {
	return r.Service.Name + "." + strings.Join(r.ResourcePath, ".") + "." + r.Method.Name
}

// CommandPath is the CLI argv segments, e.g. ["im", "chat.members", "create"].
func (r MethodRef) CommandPath() []string {
	out := make([]string, 0, len(r.ResourcePath)+2)
	out = append(out, r.Service.Name)
	out = append(out, r.ResourcePath...)
	return append(out, r.Method.Name)
}
