// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

// countingLoader records which service shards a listing actually parses, so the
// assembly scope below is asserted against real loader traffic rather than
// against the shape of the rendered output. Catalog parses distinct shards in
// parallel, so the tally is mutex-guarded rather than a bare append.
type countingLoader struct {
	names  []string
	mu     *sync.Mutex
	parsed *[]string
}

func newCountingLoader(names ...string) countingLoader {
	return countingLoader{names: names, mu: &sync.Mutex{}, parsed: &[]string{}}
}

func (l countingLoader) Names() []string { return l.names }

func (l countingLoader) Load(name string) (meta.Service, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	*l.parsed = append(*l.parsed, name)
	return meta.Service{Name: name}, nil
}

// count reports how many shards were parsed so far.
func (l countingLoader) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(*l.parsed)
}

// The bare service index is specified to cost a "None" assembly scope: zero
// service shards parsed. It holds because the listing is the manifest's own
// name set and every name's description is curated static data, so neither
// input needs a service body. Asserting the loader is never called is what
// keeps a future reader of svc.Description from silently making the cheapest
// and highest-traffic schema call walk every shard.
func TestServiceIndex_ParsesNoShardWhenNothingCanPrune(t *testing.T) {
	loader := newCountingLoader("im", "drive", "calendar")
	catalog := apicatalog.NewLazy(apicatalog.SourceEmbedded, loader)

	var out bytes.Buffer
	if err := runSchemaCatalog(&out, nil, core.StrictModeOff, catalog, nil, nil, "", nil, nil); err != nil {
		t.Fatalf("bare schema failed: %v", err)
	}

	if parsed := loader.count(); parsed != 0 {
		t.Fatalf("bare schema parsed %d shard(s), want none", parsed)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"service_index"`)) {
		t.Fatalf("want a service index, got: %s", out.String())
	}
}

// The cheap path is only sound while nothing can remove a service the manifest
// still names. A build whose plugins declare a restriction can, so it must fall
// back to the parsed listing — otherwise the index would name services the same
// build hides from root help.
func TestServiceIndex_ParsesShardsWhenConcealmentIsPossible(t *testing.T) {
	loader := newCountingLoader("im", "drive")
	catalog := apicatalog.NewLazy(apicatalog.SourceEmbedded, loader)

	var out bytes.Buffer
	visible := func([]string) bool { return true }
	conceals := func() bool { return true }
	if err := runSchemaCatalog(&out, nil, core.StrictModeOff, catalog, nil, visible, "", nil, conceals); err != nil {
		t.Fatalf("schema failed: %v", err)
	}

	if loader.count() == 0 {
		t.Fatal("a build that can conceal must parse shards to know which services survive")
	}
}
