// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

const dedupTTL = 5 * time.Minute

// PipelineConfig configures the event processing pipeline.
type PipelineConfig struct {
	Mode      TransformMode // determined by --compact flag
	JsonFlag  bool          // --json: pretty JSON instead of NDJSON
	OutputDir string        // --output-dir: write events to files
	Quiet     bool          // --quiet: suppress stderr status messages
	Router    *EventRouter  // --route: regex-based output routing
}

// EventPipeline chains filter → dedup → transform → emit.
type EventPipeline struct {
	registry   *ProcessorRegistry
	filters    *FilterChain
	config     PipelineConfig
	eventCount atomic.Int64
	seen       sync.Map // key → time.Time (first-seen timestamp)
	out        io.Writer
	errOut     io.Writer
}

// NewEventPipeline builds an event processing pipeline.
func NewEventPipeline(
	registry *ProcessorRegistry,
	filters *FilterChain,
	config PipelineConfig,
	out, errOut io.Writer,
) *EventPipeline {
	return &EventPipeline{
		registry: registry,
		filters:  filters,
		config:   config,
		out:      out,
		errOut:   errOut,
	}
}

// EnsureDirs creates all configured output directories once at startup.
func (p *EventPipeline) EnsureDirs() error {
	if p.config.OutputDir != "" {
		if err := vfs.MkdirAll(p.config.OutputDir, 0700); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}
	if p.config.Router != nil {
		for _, route := range p.config.Router.routes {
			if err := vfs.MkdirAll(route.dir, 0700); err != nil {
				return fmt.Errorf("create route dir %s: %w", route.dir, err)
			}
		}
	}
	return nil
}

// EventCount returns the number of processed events.
func (p *EventPipeline) EventCount() int64 {
	return p.eventCount.Load()
}

func (p *EventPipeline) infof(format string, args ...interface{}) {
	if !p.config.Quiet {
		fmt.Fprintf(p.errOut, format+"\n", args...)
	}
}

// isDuplicate returns true if key was seen within dedupTTL.
func (p *EventPipeline) isDuplicate(key string) bool {
	now := time.Now()
	if v, loaded := p.seen.LoadOrStore(key, now); loaded {
		if ts, ok := v.(time.Time); ok && now.Sub(ts) < dedupTTL {
			return true
		}
		p.seen.Store(key, now)
	}
	return false
}

func (p *EventPipeline) cleanupSeen(now time.Time) {
	p.seen.Range(func(k, v any) bool {
		if ts, ok := v.(time.Time); ok && now.Sub(ts) >= dedupTTL {
			p.seen.Delete(k)
		}
		return true
	})
}

// Process is the pipeline entry point, called by the WebSocket callback.
func (p *EventPipeline) Process(ctx context.Context, raw *RawEvent) {
	eventType := raw.Header.EventType

	// 1. Filter
	if !p.filters.Allow(eventType) {
		return
	}

	// 2. Lookup processor
	processor := p.registry.Lookup(eventType)

	// 3. Dedup
	if key := processor.DeduplicateKey(raw); key != "" && p.isDuplicate(key) {
		p.infof("%s[dedup]%s %s (key=%s)", output.Dim, output.Reset, eventType, key)
		return
	}

	n := p.eventCount.Add(1)
	if n%100 == 0 {
		p.cleanupSeen(time.Now())
	}

	// 4. Transform — processor returns the final serializable value
	data := processor.Transform(ctx, raw, p.config.Mode)

	// 5. Output routing (framework-controlled)
	// 5a. Route-based output — matched events go to route dirs
	if p.config.Router != nil {
		if dirs := p.config.Router.Match(eventType); len(dirs) > 0 {
			for _, dir := range dirs {
				p.writeAndLog(dir, n, eventType, data, raw.Header)
			}
			return
		}
	}

	// 5b. --output-dir
	if p.config.OutputDir != "" {
		p.writeAndLog(p.config.OutputDir, n, eventType, data, raw.Header)
		return
	}

	// 5c. Stdout
	if p.config.JsonFlag {
		output.PrintJson(p.out, data)
	} else {
		output.PrintNdjson(p.out, data)
	}
	p.infof("%s[%d]%s %s", output.Dim, n, output.Reset, eventType)
}

// writeAndLog writes an event to a directory and logs the result.
func (p *EventPipeline) writeAndLog(dir string, n int64, eventType string, data interface{}, header larkevent.EventHeader) {
	fp, err := writeEventFile(dir, data, header)
	if err != nil {
		output.PrintError(p.errOut, fmt.Sprintf("write failed (%s): %v", dir, err))
	} else {
		p.infof("%s[%d]%s %s → %s", output.Dim, n, output.Reset, eventType, fp)
	}
}

var filenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func writeEventFile(dir string, data interface{}, header larkevent.EventHeader) (string, error) {
	eventID := header.EventID
	if eventID == "" {
		eventID = "unknown"
	}
	ts := header.CreateTime
	if ts == "" {
		ts = fmt.Sprintf("%d", os.Getpid())
	}

	safeName := filenameSanitizer.ReplaceAllString(header.EventType, "_")
	filename := fmt.Sprintf("%s_%s_%s.json", safeName, eventID, ts)
	outPath := filepath.Join(dir, filename)

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	if err := validate.AtomicWrite(outPath, append(jsonData, '\n'), 0600); err != nil {
		return "", err
	}

	return outPath, nil
}
