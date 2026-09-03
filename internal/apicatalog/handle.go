// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog

import (
	"errors"
	"sort"
	"sync"

	"github.com/larksuite/cli/internal/meta"
)

// ErrServiceNotFound is returned by a Loader whose Names listed a service that
// it cannot provide after all (for example a projection that removed every
// reachable method). The Catalog treats it as "absent", not as a failure.
var ErrServiceNotFound = errors.New("apicatalog: service not found")

// Loader is the per-service data source behind a lazy Catalog. Names must be
// cheap and must not parse service bodies; Load parses exactly one service and
// owns its integrity: a successful Load returns a service whose Name is the
// requested name. Any error other than ErrServiceNotFound is a source failure
// that the Catalog records (see Catalog.Err) and surfaces through Preload.
type Loader interface {
	Names() []string
	Load(name string) (meta.Service, error)
}

// Catalog is a cheap-to-copy navigation handle over services. Copies share one
// lazily populated state, so a service body is parsed at most once per Catalog
// no matter how many holders navigate it. The zero Catalog is empty.
//
// Navigation is lenient by design: Service reports a failed shard as absent,
// and Services, WalkMethods, Resolve, and completion skip it, so help and
// completion keep working for the rest of the catalog. A caller whose result
// must not silently shrink when a shard is corrupt — anything that persists,
// authorizes, or enumerates "everything" — calls Preload for the names it is
// about to consult (or checks Err after enumerating) and returns that typed
// error instead of continuing.
type Catalog struct{ *catalogState }

type catalogState struct {
	source Source
	loader Loader

	mu     sync.Mutex
	names  []string // sorted; from loader.Names, resolved once
	loaded map[string]*loadedService
	all    []meta.Service // every resolvable service in name order, once enumerated
	err    error          // first non-ErrServiceNotFound Load failure
}

// loadedService is one shard's parse, performed at most once. The Once runs
// Loader.Load outside the catalog mutex so distinct shards can parse in
// parallel while a shard requested twice still parses once.
type loadedService struct {
	once    sync.Once
	service meta.Service
	err     error
}

// New builds an eager Catalog over the given services. The slice is copied and
// sorted by name so callers may pass any order. The copy is shallow —
// meta.Service values share their Resources maps, which are treated as
// read-only.
func New(source Source, services []meta.Service) Catalog {
	sorted := append([]meta.Service(nil), services...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	names := make([]string, len(sorted))
	loaded := make(map[string]*loadedService, len(sorted))
	for i, s := range sorted {
		names[i] = s.Name
		entry := &loadedService{service: s}
		entry.once.Do(func() {})
		loaded[s.Name] = entry
	}
	return Catalog{&catalogState{
		source: source,
		loader: sliceLoader(names),
		names:  names,
		loaded: loaded,
		all:    sorted,
	}}
}

// NewLazy builds a Catalog that parses services on first use through loader.
func NewLazy(source Source, loader Loader) Catalog {
	return Catalog{&catalogState{source: source, loader: loader}}
}

// sliceLoader backs an eager Catalog: every service is already loaded, so Load
// is only reached for unknown names.
type sliceLoader []string

func (l sliceLoader) Names() []string                 { return l }
func (sliceLoader) Load(string) (meta.Service, error) { return meta.Service{}, ErrServiceNotFound }

// Filter derives a Catalog whose services are the result of applying keep to
// each service of c on first use. keep returns the (possibly reduced) service
// and false to drop it. The derived Catalog shares no cache with c beyond what
// c itself has already parsed.
func Filter(c Catalog, keep func(meta.Service) (meta.Service, bool)) Catalog {
	if c.catalogState == nil {
		return Catalog{}
	}
	return NewLazy(c.source, filterLoader{parent: c, keep: keep})
}

type filterLoader struct {
	parent Catalog
	keep   func(meta.Service) (meta.Service, bool)
}

func (l filterLoader) Names() []string { return l.parent.Names() }

func (l filterLoader) Load(name string) (meta.Service, error) {
	svc, err := l.parent.load(name)
	if err != nil {
		return meta.Service{}, err
	}
	kept, ok := l.keep(svc)
	if !ok {
		return meta.Service{}, ErrServiceNotFound
	}
	return kept, nil
}

// Source reports the catalog origin.
func (c Catalog) Source() Source {
	if c.catalogState == nil {
		return ""
	}
	return c.source
}

// Names returns the sorted names the underlying loader can provide, without
// parsing any service body. A projected service may still be absent from
// Services; callers that need the exact resolvable set use Services.
func (c Catalog) Names() []string {
	if c.catalogState == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.namesLocked()...)
}

func (s *catalogState) namesLocked() []string {
	if s.names == nil {
		names := append([]string(nil), s.loader.Names()...)
		sort.Strings(names)
		s.names = names
	}
	return s.names
}

// Services returns every resolvable service in name order, loading any that
// have not been parsed yet. Treat the result as read-only: it is the Catalog's
// own ordered slice and its element Resources maps are shared. Services whose
// Load failed are omitted; the failure is retained for Err.
func (c Catalog) Services() []meta.Service {
	if c.catalogState == nil {
		return nil
	}
	c.mu.Lock()
	if c.all != nil {
		all := c.all
		c.mu.Unlock()
		return all
	}
	names := append([]string(nil), c.namesLocked()...)
	c.mu.Unlock()

	c.loadAll(names)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.all == nil {
		all := make([]meta.Service, 0, len(names))
		for _, name := range names {
			if entry := c.loaded[name]; entry != nil && entry.err == nil {
				all = append(all, entry.service)
			}
		}
		c.all = all
	}
	return c.all
}

// Service looks up one service by name, parsing it on first use.
func (c Catalog) Service(name string) (meta.Service, bool) {
	svc, err := c.load(name)
	return svc, err == nil
}

// Preload parses the named services now, distinct shards in parallel, and
// returns the first failure. Build paths use it so a corrupt service surfaces
// as a typed error before any command is dispatched; afterwards Service and
// Services for those names are pure cache hits.
func (c Catalog) Preload(names ...string) error {
	if c.catalogState == nil {
		return nil
	}
	c.loadAll(names)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range names {
		if entry := c.loaded[name]; entry != nil && entry.err != nil && !errors.Is(entry.err, ErrServiceNotFound) {
			return entry.err
		}
	}
	return nil
}

// Err returns the first source failure observed while loading services (never
// ErrServiceNotFound). Enumeration paths that must not hide a corrupt source
// check it after Services or WalkMethods.
func (c Catalog) Err() error {
	if c.catalogState == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c Catalog) load(name string) (meta.Service, error) {
	if c.catalogState == nil {
		return meta.Service{}, ErrServiceNotFound
	}
	entry := c.entry(name)
	if entry == nil {
		return meta.Service{}, ErrServiceNotFound
	}
	c.parse(entry, name)
	return entry.service, entry.err
}

// loadAll parses every named shard that is not yet loaded, in parallel.
func (c Catalog) loadAll(names []string) {
	var wg sync.WaitGroup
	for _, name := range names {
		entry := c.entry(name)
		if entry == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.parse(entry, name)
		}()
	}
	wg.Wait()
}

// entry returns the load slot for a known name, creating it on first request,
// or nil for a name the loader does not list.
func (s *catalogState) entry(name string) *loadedService {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.loaded[name]; ok {
		return entry
	}
	if !s.knownLocked(name) {
		return nil
	}
	if s.loaded == nil {
		s.loaded = make(map[string]*loadedService)
	}
	entry := &loadedService{}
	s.loaded[name] = entry
	return entry
}

// parse runs the loader for entry exactly once and records the first source
// failure on the catalog.
func (s *catalogState) parse(entry *loadedService, name string) {
	entry.once.Do(func() {
		svc, err := s.loader.Load(name)
		if err != nil {
			svc = meta.Service{}
			if !errors.Is(err, ErrServiceNotFound) {
				s.mu.Lock()
				if s.err == nil {
					s.err = err
				}
				s.mu.Unlock()
			}
		}
		entry.service, entry.err = svc, err
	})
}

func (s *catalogState) knownLocked(name string) bool {
	names := s.namesLocked()
	i := sort.SearchStrings(names, name)
	return i < len(names) && names[i] == name
}
