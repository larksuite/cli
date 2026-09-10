// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package affordance is the lazily-loaded store of usage guidance for
// service-API methods. The source of truth is one markdown file per service in
// the top-level affordance/ tree (see mdparse.go), injected via SetSource so
// domain owners maintain it next to skills/ and shortcuts/.
//
// Guidance is keyed by method id, but the markdown headings use the command
// form ("user_mailbox messages list"), and mapping one to the other needs the
// API catalog that built the command tree. A Resolver therefore belongs to one
// build: it pairs the content tree with that build's Catalog and resolves each
// service at most once, so normal command execution never touches the
// markdown and help rendering never rebuilds a mapping it already has.
package affordance

import (
	"encoding/json"
	"io/fs"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/apicatalog"
)

var (
	sourceMu sync.Mutex
	mdSource fs.FS // top-level affordance/*.md tree; nil in the minimal preview build
)

// SetSource installs the markdown guidance tree (the top-level affordance/
// directory) as the content source for Resolvers created afterwards. Called
// once at startup before any lookup.
func SetSource(fsys fs.FS) {
	sourceMu.Lock()
	defer sourceMu.Unlock()
	mdSource = fsys
}

// Source returns the registered markdown guidance tree, or nil when the build
// embeds none.
func Source() fs.FS {
	sourceMu.Lock()
	defer sourceMu.Unlock()
	return mdSource
}

// Resolver serves guidance for one build: one content tree paired with the
// Catalog whose command forms the headings are written against. Each service
// is read, parsed, and mapped once, on first access.
type Resolver struct {
	source  fs.FS
	catalog apicatalog.Catalog

	mu        sync.Mutex
	byService map[string]parsedDomain
}

// NewResolver pairs a guidance tree with the catalog that built the command
// tree. A nil source yields a Resolver that reports no guidance.
func NewResolver(source fs.FS, catalog apicatalog.Catalog) *Resolver {
	return &Resolver{source: source, catalog: catalog, byService: map[string]parsedDomain{}}
}

// For returns the raw affordance overlay for one method, loading the owning
// service on first access. ok is false when there is no entry (absent source,
// parse failure, or unknown method all collapse to "no guidance"). A nil
// Resolver reports no guidance.
func (r *Resolver) For(service, methodID string) (json.RawMessage, bool) {
	if r == nil {
		return nil, false
	}
	parsed, ok := r.domain(service)
	if !ok {
		return nil, false
	}
	raw, ok := parsed.raw[methodID]
	return raw, ok && len(raw) > 0
}

// DomainSkill returns the service-level canonical skill declared by
// `> skill:`. That declaration is independent of method command mappings.
func (r *Resolver) DomainSkill(service string) (string, bool) {
	if r == nil {
		return "", false
	}
	parsed, ok := r.domain(service)
	if !ok {
		return "", false
	}
	return parsed.skill, parsed.skill != ""
}

// DomainSkills returns the skill references configured for service-level help.
// The canonical `> skill:` entry is first when present, followed by entries in
// the domain's `## Skills` section. The returned slice is a copy so callers
// cannot mutate the cache.
func (r *Resolver) DomainSkills(service string) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	parsed, ok := r.domain(service)
	if !ok {
		return nil, false
	}
	if len(parsed.domainSkills) == 0 {
		return nil, false
	}
	return append([]string(nil), parsed.domainSkills...), true
}

// domain returns the parsed, catalog-mapped guidance for one service, reading
// and resolving it on first access. A missing file is cached as absent so a
// domain without guidance is stat'ed once, not on every help render.
func (r *Resolver) domain(service string) (parsedDomain, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if parsed, ok := r.byService[service]; ok {
		return parsed, parsed.present
	}
	parsed := parsedDomain{}
	if r.source != nil {
		if src, err := fs.ReadFile(r.source, service+".md"); err == nil {
			parsed = parseDomainMD(src, commandFormResolver(r.catalog, service))
			parsed.present = true
			// Help and skill-reference lookups ask for the wire form repeatedly;
			// encode each method once here rather than on every For call.
			parsed.raw = make(map[string]json.RawMessage, len(parsed.methods))
			for id, a := range parsed.methods {
				if raw, err := json.Marshal(a); err == nil {
					parsed.raw[id] = raw
				}
			}
		}
	}
	r.byService[service] = parsed
	return parsed, parsed.present
}

// commandFormResolver maps a method's command-form heading ("user_mailbox.messages
// list") to its method id ("user_mailbox.message.list") via the catalog's
// authoritative resource↔id table. Resource names are irregularly pluralised
// (message/messages, user_mailbox/user_mailboxes), so this cannot be guessed; the
// space→dot fallback covers domains where the two already coincide.
func commandFormResolver(catalog apicatalog.Catalog, service string) func(string) string {
	byForm := map[string]string{}
	if svc, ok := catalog.Service(service); ok {
		for _, ref := range apicatalog.ServiceMethods(svc, nil) {
			byForm[strings.Join(ref.CommandPath()[1:], " ")] = ref.Method.ID
		}
	}
	return func(h string) string {
		if id, ok := byForm[strings.TrimSpace(h)]; ok {
			return id
		}
		return headingToKey(h) // one home for the shortcut/method key convention
	}
}
