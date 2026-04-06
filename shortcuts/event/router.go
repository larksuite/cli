// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/larksuite/cli/internal/validate"
)

// Route holds a compiled regex pattern and its target output directory.
type Route struct {
	pattern *regexp.Regexp
	dir     string
}

// EventRouter dispatches events to output directories by regex matching on event_type.
type EventRouter struct {
	routes []Route
}

// ParseRoutes parses route flag values into an EventRouter.
// Format: "regex=dir:./path/to/dir"
// Returns nil, nil when input is empty.
func ParseRoutes(specs []string) (*EventRouter, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	routes := make([]Route, 0, len(specs))
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid route %q: expected format regex=dir:./path", spec)
		}
		pattern := parts[0]
		target := parts[1]

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in route %q: %w", spec, err)
		}

		if !strings.HasPrefix(target, "dir:") {
			return nil, fmt.Errorf("invalid route target %q: must start with \"dir:\" prefix (format: regex=dir:./path)", target)
		}
		dir := strings.TrimPrefix(target, "dir:")
		if dir == "" {
			return nil, fmt.Errorf("invalid route %q: directory path is empty", spec)
		}

		safeDir, err := validate.SafeOutputPath(dir)
		if err != nil {
			return nil, fmt.Errorf("invalid route %q: %w", spec, err)
		}

		routes = append(routes, Route{pattern: re, dir: safeDir})
	}

	return &EventRouter{routes: routes}, nil
}

// Match returns all target directories for the given event type.
// Returns nil if no routes match (caller should fall through to default output).
func (r *EventRouter) Match(eventType string) []string {
	var dirs []string
	for _, route := range r.routes {
		if route.pattern.MatchString(eventType) {
			dirs = append(dirs, route.dir)
		}
	}
	return dirs
}

// OutputRecordWriter writes a fully serialized pipeline record.
type OutputRecordWriter interface {
	WriteRecord(eventType string, record map[string]interface{}) error
}

type outputRouter struct {
	router     *EventRouter
	defaultDir string
	fallback   OutputRecordWriter
	seq        *uint64
	writers    map[string]*dirRecordWriter
	mu         sync.Mutex
}

func (r *outputRouter) WriteRecord(eventType string, record map[string]interface{}) error {
	if r == nil {
		return nil
	}

	dirs := r.matchDirs(eventType)
	if len(dirs) == 0 {
		if r.fallback == nil {
			return nil
		}
		return r.fallback.WriteRecord(eventType, record)
	}

	for _, dir := range dirs {
		writer := r.writerForDir(dir)
		if err := writer.WriteRecord(eventType, record); err != nil {
			return err
		}
	}
	return nil
}

func (r *outputRouter) writerForDir(dir string) *dirRecordWriter {
	r.mu.Lock()
	defer r.mu.Unlock()

	writer := r.writers[dir]
	if writer == nil {
		writer = &dirRecordWriter{dir: dir, seq: r.seq}
		r.writers[dir] = writer
	}
	return writer
}

func (r *outputRouter) matchDirs(eventType string) []string {
	if r == nil {
		return nil
	}
	var dirs []string
	if r.router != nil {
		dirs = append(dirs, r.router.Match(eventType)...)
	}
	if len(dirs) == 0 && r.defaultDir != "" {
		dirs = append(dirs, r.defaultDir)
	}
	return dirs
}

type dirRecordWriter struct {
	dir string
	seq *uint64
}

func (w *dirRecordWriter) WriteRecord(_ string, record map[string]interface{}) error {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	name := w.nextFileName(record)
	return validate.AtomicWrite(filepath.Join(w.dir, name), append(data, '\n'), 0o600)
}

func (w *dirRecordWriter) nextFileName(record map[string]interface{}) string {
	seq := atomic.AddUint64(w.seq, 1)
	eventType, _ := record["event_type"].(string)
	eventID, _ := record["event_id"].(string)
	if eventID == "" {
		eventID, _ = record["idempotency_key"].(string)
	}
	base := sanitizeRouteFilePart(eventType)
	if base == "" {
		base = "event"
	}
	id := sanitizeRouteFilePart(eventID)
	if id == "" {
		return fmt.Sprintf("%06d-%s.json", seq, base)
	}
	return fmt.Sprintf("%06d-%s-%s.json", seq, base, id)
}

func sanitizeRouteFilePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, string(filepath.Separator), "-")
	value = strings.ReplaceAll(value, "/", "-")
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}
