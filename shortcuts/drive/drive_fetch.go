// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

// DriveFetch reads supported Lark resources as Markdown with a unified output
// envelope. It detects the URL type, unwraps Wiki links, and dispatches to the
// document, content-read, or Minutes API as appropriate.
var DriveFetch = common.Shortcut{
	Service:     "drive",
	Command:     "+fetch",
	Description: "Fetch any Lark doc/sheet/base/slides/file/minutes as a readable markdown snapshot (auto-detects type; unwraps wiki)",
	Risk:        "read",
	Scopes:      []string{},
	// Each resource type checks scopes at dispatch time. Conditional scopes expose
	// complete auth metadata without preflighting unrelated resource types.
	ConditionalUserScopes: []string{
		"docx:document:readonly",
		"wiki:node:retrieve",
		"minutes:minutes.basic:read",
		"minutes:minutes.artifacts:read",
		"vc:note:read",
	},
	ConditionalBotScopes: []string{
		"docx:document:readonly",
		"wiki:node:retrieve",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "url", Desc: "Lark/Feishu resource URL (docx, doc, sheet, base, wiki, slides, file, minutes)"},
		{Name: "token", Desc: "bare resource token (requires --type)"},
		{Name: "type", Enum: []string{"doc", "docx", "sheet", "sheets", "base", "bitable", "slides", "file", "minutes", "wiki"}, Desc: "resource type (required with --token; auto-detected for --url)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "cap each rendered table to N data rows (0 = no limit)"},
		{Name: "full", Type: "bool", Default: "false", Desc: "return the whole resource content in one response (disable auto-pagination; not for minutes)"},
		{Name: "page-token", Desc: "continue a paginated read from a prior next_page_token (not for minutes)"},
		{Name: "page-size", Type: "int", Default: "0", Desc: "per-page token budget hint (0 = server default; not for minutes)"},
		{Name: "include", Desc: "minutes only: comma-separated extras to append: transcript, note-doc"},
	},
	Tips: []string{
		"Unified read entry: pass any Lark doc/sheet/base/slides/file/minutes URL (or --token --type) and get a readable markdown snapshot.",
		"For doc deep-read with --scope/--detail use `docs +fetch`; for structured sheet/base data use `sheets +cells-get` / `base +record-list`.",
		"Wiki links are unwrapped to the underlying resource and read directly; the originating wiki node is recorded in resource.source.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateFetch(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return PlanFetchDryRun(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return RunFetch(ctx, runtime)
	},
}

// RunFetch resolves input, unwraps Wiki, dispatches by type, and emits output.
func RunFetch(ctx context.Context, runtime *common.RuntimeContext) error {
	in, err := resolveDriveFetchInput(runtime)
	if err != nil {
		return err
	}
	brand := runtime.Config.Brand

	// Unwrap wiki → underlying obj_type/obj_token (and record wiki provenance).
	fetchType := in.inputType
	fetchToken := in.token
	var wikiSrc *fetchSource
	if in.inputType == "wiki" {
		fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(fetchToken))
		node, werr := common.ResolveWikiNode(runtime, fetchToken)
		if werr != nil {
			// get_node failed (e.g. the user identity lacks wiki:node:retrieve
			// scope, or the node is not found). The fetch service reads the wiki
			// URL directly server-side (it unwraps the node itself) and paginates
			// by wiki URL + page_token — independent of get_node's obj_type — so
			// fall back to direct fetch and honor any --page-token/--full the
			// caller passed (fetchWikiDirect surfaces has_more/next_page_token).
			fmt.Fprintf(runtime.IO().ErrOut,
				"[fetch] wiki get_node failed (%v); falling back to direct fetch of the wiki URL\n", werr)
			out, ferr := fetchWikiDirect(ctx, runtime, in)
			if ferr != nil {
				return ferr
			}
			res := fetchResource{
				Type:       "wiki",
				Title:      out.title,
				Token:      in.token,
				URL:        in.rawURL,
				Selector:   in.selector,
				UpdateTime: out.updateTime,
				Source:     &fetchSource{Type: "wiki", InputURL: in.rawURL},
			}
			return emitDriveFetch(runtime, out, res)
		}
		objType, ok := normalizeFetchType(node.ObjType)
		if !ok {
			return common.ValidationErrorf("wiki node resolved to %q, which is not a fetchable resource type", node.ObjType).WithParam("--url")
		}
		fetchType = objType
		fetchToken = node.ObjToken
		wikiSrc = &fetchSource{Type: "wiki", InputURL: in.rawURL, NodeToken: node.NodeToken, SpaceID: node.SpaceID}
		if err := validateFetchTypeFlags(runtime, fetchType); err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Wiki unwrapped to %s: %s\n", fetchType, common.MaskToken(fetchToken))
	}

	out, err := dispatchDriveFetch(ctx, runtime, in, fetchType, fetchToken, wikiSrc != nil)
	if err != nil {
		return err
	}

	res := fetchResource{
		Type:             fetchType,
		Title:            out.title,
		Token:            fetchToken,
		Selector:         in.selector,
		UpdateTime:       out.updateTime,
		CreateTime:       out.createTime,
		NoteID:           out.noteID,
		NoteDocToken:     out.noteDocToken,
		VerbatimDocToken: out.verbatimDocToken,
		Source:           wikiSrc,
	}
	res.URL = fetchResourceURL(brand, in, fetchType, fetchToken, wikiSrc != nil)
	return emitDriveFetch(runtime, out, res)
}

func emitDriveFetch(runtime *common.RuntimeContext, out *driveFetchOutput, res fetchResource) error {
	warnings := append([]string(nil), out.warnings...)
	cursorHint := contentread.PaginationCursorHint(out.hasMore, out.nextToken)
	if cursorHint != "" {
		warnings = append(warnings, cursorHint)
	}
	env := newFetchEnvelope(out.content, res).
		withPagination(out.hasMore, out.nextToken).
		withWarnings(warnings...)
	delivery, scan, err := common.PrepareFetchContentDelivery(runtime, env, out.content, ".data.content")
	if err != nil {
		return err
	}
	emitted := env.withContentDelivery(delivery)
	if cursorHint != "" && runtime.Format != "pretty" {
		fmt.Fprintf(runtime.IO().ErrOut, "[fetch] warning: %s\n", cursorHint)
	}
	runtime.OutFormatRawWithSafety(emitted, nil, func(w io.Writer) {
		writeDriveFetchPretty(w, delivery, emitted.Resource, emitted.Warnings)
	}, scan)
	return nil
}

func writeDriveFetchPretty(w io.Writer, delivery common.FetchContentDelivery, resource fetchResource, warnings []string) {
	common.WriteFetchContentPretty(w, delivery)
	if resource.NoteID != "" || resource.NoteDocToken != "" || resource.VerbatimDocToken != "" {
		fmt.Fprintln(w, "\nRelated note:")
		if resource.NoteID != "" {
			fmt.Fprintf(w, "  note_id: %s\n", resource.NoteID)
		}
		if resource.NoteDocToken != "" {
			fmt.Fprintf(w, "  note_doc_token: %s\n", resource.NoteDocToken)
		}
		if resource.VerbatimDocToken != "" {
			fmt.Fprintf(w, "  verbatim_doc_token: %s\n", resource.VerbatimDocToken)
		}
	}
	if len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warning := range warnings {
			fmt.Fprintf(w, "- %s\n", warning)
		}
	}
}

type driveFetchOutput struct {
	content          string
	title            string
	updateTime       int64
	createTime       string // minutes only
	noteID           string // --include note-doc only
	noteDocToken     string // --include note-doc only
	verbatimDocToken string // --include note-doc only
	hasMore          bool
	nextToken        string
	warnings         []string
}
