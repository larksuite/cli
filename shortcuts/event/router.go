// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"regexp"
	"strings"

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
			return nil, eventValidationParamError("--route", "invalid --route %q: expected format regex=dir:./path", spec)
		}
		pattern := parts[0]
		target := parts[1]

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, eventValidationParamErrorWithCause(err, "--route", "invalid regex in --route %q", spec)
		}

		if !strings.HasPrefix(target, "dir:") {
			return nil, eventValidationParamError("--route", "invalid --route target %q: must start with \"dir:\" prefix (format: regex=dir:./path)", target)
		}
		dir := strings.TrimPrefix(target, "dir:")
		if dir == "" {
			return nil, eventValidationParamError("--route", "invalid --route %q: directory path is empty", spec)
		}

		safeDir, err := validate.SafeOutputPath(dir)
		if err != nil {
			return nil, eventValidationParamErrorWithCause(err, "--route", "invalid --route %q", spec)
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
