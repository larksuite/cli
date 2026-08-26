// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"html"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveCitationHighlightTagRE = regexp.MustCompile(`</?hb?>`)

type driveSearchCitationItem struct {
	EntityType string
	DocType    string
	Title      string
	URL        string
	Token      string
	CreateTime string
}

func projectDriveSearchCitationItem(raw map[string]interface{}) driveSearchCitationItem {
	meta := common.GetMap(raw, "result_meta")
	title := strings.TrimSpace(common.GetString(raw, "title"))
	if title == "" {
		highlightedTitle := driveCitationHighlightTagRE.ReplaceAllString(common.GetString(raw, "title_highlighted"), "")
		title = strings.TrimSpace(html.UnescapeString(highlightedTitle))
	}
	return driveSearchCitationItem{
		EntityType: strings.ToUpper(common.GetString(raw, "entity_type")),
		DocType:    strings.ToUpper(common.GetString(meta, "doc_types")),
		Title:      title,
		URL:        strings.TrimSpace(common.GetString(meta, "url")),
		Token:      common.GetString(meta, "token"),
		CreateTime: common.GetStringLoose(meta, "create_time"),
	}
}

func driveSearchCitationSource(item driveSearchCitationItem) citation.SourceType {
	if item.EntityType == "WIKI" {
		return citation.SourceWiki
	}
	switch item.DocType {
	case "SHEET":
		return citation.SourceSheet
	case "BITABLE":
		return citation.SourceBase
	case "DOC", "DOCX":
		return citation.SourceDoc
	case "MINDNOTE":
		return citation.SourceMindnote
	case "SLIDES":
		return citation.SourceSlides
	case "FILE":
		return citation.SourceFile
	}
	return citation.SourceDoc
}

func driveSearchCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	items, ok := out["results"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]citation.Citation, 0, len(items))
	for _, raw := range items {
		itemMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		item := projectDriveSearchCitationItem(itemMap)
		result = append(result, citation.Citation{
			SourceType:  driveSearchCitationSource(item),
			URL:         item.URL,
			Title:       item.Title,
			ResourceID:  item.Token,
			PublishTime: citation.Time(item.CreateTime),
		})
	}
	return result
}

type driveInspectCitationItem struct {
	DocType    string
	Title      string
	URL        string
	Token      string
	CreateTime string
}

func projectDriveInspectCitationItem(raw map[string]interface{}) driveInspectCitationItem {
	return driveInspectCitationItem{
		DocType:    strings.ToLower(common.GetString(raw, "type")),
		Title:      common.GetString(raw, "title"),
		URL:        strings.TrimSpace(common.GetString(raw, "url")),
		Token:      common.GetString(raw, "token"),
		CreateTime: common.GetStringLoose(raw, "create_time"),
	}
}

func driveInspectCitationSource(docType string) citation.SourceType {
	switch strings.ToLower(docType) {
	case "sheet":
		return citation.SourceSheet
	case "bitable":
		return citation.SourceBase
	case "doc", "docx":
		return citation.SourceDoc
	case "mindnote":
		return citation.SourceMindnote
	case "slides":
		return citation.SourceSlides
	case "file":
		return citation.SourceFile
	}
	return citation.SourceDoc
}

func driveInspectCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	item := projectDriveInspectCitationItem(out)
	return []citation.Citation{{
		SourceType:  driveInspectCitationSource(item.DocType),
		URL:         item.URL,
		Title:       item.Title,
		ResourceID:  item.Token,
		PublishTime: citation.Time(item.CreateTime),
	}}
}
