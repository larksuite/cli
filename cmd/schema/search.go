// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/schema"
)

type searchReport struct {
	Query   string                `json:"query"`
	Total   int                   `json:"total"`
	Offset  int                   `json:"offset"`
	Limit   int                   `json:"limit"`
	HasMore bool                  `json:"has_more"`
	Results []schema.SearchResult `json:"results"`
}

func validateSearchOptions(opts *SchemaOptions) error {
	if !opts.SearchSet {
		for _, flag := range []struct {
			name    string
			changed bool
		}{{"--limit", opts.LimitSet}, {"--offset", opts.OffsetSet}} {
			if flag.changed {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s requires --search", flag.name).
					WithParam(flag.name).WithHint("use --search to discover methods, or omit pagination flags for full schemas")
			}
		}
		return nil
	}
	if strings.TrimSpace(opts.Search) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "search query must not be empty").WithParam("--search")
	}
	if opts.Limit < 1 || opts.Limit > 100 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "search limit must be between 1 and 100").WithParam("--limit")
	}
	if opts.Offset < 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "search offset must not be negative").WithParam("--offset")
	}
	return nil
}

func schemaSearchRun(opts *SchemaOptions, visibility CommandVisibility) error {
	catalog := opts.Factory.APICatalog
	if len(catalog.Names()) == 0 {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "No API metadata available").
			WithHint("the current command build did not select any API metadata")
	}
	catalog = projectSchemaCatalog(catalog, visibility)
	target, err := catalog.Resolve(apicatalog.ParsePath(opts.Args))
	if err != nil {
		if loadErr := catalog.Err(); loadErr != nil {
			return loadErr
		}
		return resolveError(err)
	}
	mode := opts.Factory.ResolveStrictMode(opts.Ctx)
	refs := catalog.MethodRefs(target, registry.FilterForStrictMode(mode))
	// Search must not turn a damaged metadata shard into a successful partial
	// result, even when a result limit would otherwise hide that shard.
	if loadErr := catalog.Err(); loadErr != nil {
		return loadErr
	}
	matches := apicatalog.SearchMethods(refs, opts.Search)
	start := min(opts.Offset, len(matches))
	// Clamp before adding: --offset may be MaxInt.
	end := start + min(opts.Limit, len(matches)-start)
	report := searchReport{
		Query: opts.Search, Total: len(matches), Offset: opts.Offset, Limit: opts.Limit,
		HasMore: end < len(matches), Results: schema.SearchResults(matches[start:end]),
	}
	return output.NewEmitter(output.EmitterConfig{
		Out: opts.Factory.IOStreams.Out, ErrOut: opts.Factory.IOStreams.ErrOut,
		CommandPath: "schema",
	}).Success(report, output.EmitOptions{})
}
