// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package affordance is the lazily-loaded store of usage guidance for
// service-API methods. The source of truth is one markdown file per service in
// the top-level affordance/ tree (see mdparse.go), injected via SetSource so
// domain owners maintain it next to skills/ and shortcuts/. A service is read
// and parsed at most once, on first access, so normal command execution never
// touches it.
package affordance

import (
	"encoding/json"
	"io/fs"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/registry"
)

var (
	mu        sync.Mutex
	byService = map[string]serviceAffordance{}
	tried     = map[string]bool{}
	mdSource  fs.FS // top-level affordance/*.md tree; nil in the minimal preview build
)

type serviceAffordance struct {
	skill   string
	methods map[string]json.RawMessage
}

// SetSource installs the markdown guidance tree (the top-level affordance/
// directory) as the source. Called once at startup before any lookup; clears
// the parse cache so re-sourcing (e.g. in tests) takes effect.
func SetSource(fsys fs.FS) {
	mu.Lock()
	defer mu.Unlock()
	mdSource = fsys
	byService = map[string]serviceAffordance{}
	tried = map[string]bool{}
}

// For returns the raw affordance overlay for one method, loading the owning
// service on first access. ok is false when there is no entry (absent source,
// parse failure, or unknown method all collapse to "no guidance").
func For(service, methodID string) (json.RawMessage, bool) {
	mu.Lock()
	defer mu.Unlock()
	if !tried[service] {
		tried[service] = true
		byService[service] = loadService(service)
	}
	raw, ok := byService[service].methods[methodID]
	return raw, ok && len(raw) > 0
}

// DomainSkill returns the service-level canonical skill declared by
// `> skill:`. It shares For's lazy parse cache, so domain and command help
// cannot disagree about the declared skill.
func DomainSkill(service string) (string, bool) {
	mu.Lock()
	defer mu.Unlock()
	if !tried[service] {
		tried[service] = true
		byService[service] = loadService(service)
	}
	skill := byService[service].skill
	return skill, skill != ""
}

// loadService parses a service's markdown guidance into its domain metadata
// and per-method overlays, marshalling each method to JSON so downstream
// callers keep the same wire shape.
func loadService(service string) serviceAffordance {
	if mdSource == nil {
		return serviceAffordance{}
	}
	src, err := fs.ReadFile(mdSource, service+".md")
	if err != nil {
		return serviceAffordance{}
	}
	m := map[string]json.RawMessage{}
	parsed := parseDomainMD(src, commandFormResolver(service))
	for id, a := range parsed.methods {
		if b, err := json.Marshal(a); err == nil {
			m[id] = b
		}
	}
	return serviceAffordance{skill: parsed.skill, methods: m}
}

// commandFormResolver maps a method's command-form heading ("user_mailbox.messages
// list") to its method id ("user_mailbox.message.list") via the registry's
// authoritative resource↔id table. Resource names are irregularly pluralised
// (message/messages, user_mailbox/user_mailboxes), so this cannot be guessed; the
// space→dot fallback covers domains where the two already coincide.
func commandFormResolver(service string) func(string) string {
	byForm := map[string]string{}
	if svc, ok := registry.SchemaCatalog().Service(service); ok {
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
