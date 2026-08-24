// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	wikiCitationMetadataScope        = "drive:drive.metadata:readonly"
	wikiCitationMetaBatchMaxRequests = 200
)

type wikiCitationMeta struct {
	NodeToken string
	URL       string
}

// wikiCitationPayload carries metadata used only by Citation.Build while
// serializing exactly the original command data. Data remains exported so a
// content-safety provider that inspects values instead of JSON can still scan
// every business text field; URLs contains no citation text and is excluded
// from the wire payload.
type wikiCitationPayload struct {
	Data any               `json:"-"`
	URLs map[string]string `json:"-"`
}

func (p wikiCitationPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Data)
}

// wikiCitationEnvelopeRequested mirrors the output branches used by these two
// Wiki shortcuts: jq always filters the JSON envelope, while an empty/json
// format emits the envelope directly. Their pretty renderers and record
// formats never carry citations, so they must not pay for metadata lookup.
func wikiCitationEnvelopeRequested(runtime *common.RuntimeContext) bool {
	if runtime == nil || !citation.Enabled() {
		return false
	}
	if runtime.JqExpr != "" {
		return true
	}
	switch strings.ToLower(runtime.Format) {
	case "pretty", "table", "csv", "ndjson":
		return false
	default:
		// Empty/json and unknown formats all reach the JSON envelope; the
		// emitter warns and falls back to JSON for the unknown-format case.
		return true
	}
}

// wikiPayloadParts returns the public command data and the private native-URL
// lookup carried beside it. Plain payloads remain supported so builders fail
// closed to URL-less citations when called with an unexpected shape.
func wikiPayloadParts(data any) (any, map[string]string) {
	switch payload := data.(type) {
	case wikiCitationPayload:
		return payload.Data, payload.URLs
	case *wikiCitationPayload:
		if payload != nil {
			return payload.Data, payload.URLs
		}
	}
	return data, nil
}

// fetchWikiCitationURLs resolves Wiki node tokens to tenant-native URLs using
// Drive Meta's native batch API. The 200-item chunk size is the OpenAPI limit,
// not a CLI policy. This Wiki-owned adapter stays private so existing Drive
// shortcuts keep their established common.FetchDriveMeta execution path.
func fetchWikiCitationURLs(runtime *common.RuntimeContext, nodeTokens []string) (map[string]string, error) {
	unique := make([]string, 0, len(nodeTokens))
	seen := make(map[string]struct{}, len(nodeTokens))
	for _, nodeToken := range nodeTokens {
		nodeToken = strings.TrimSpace(nodeToken)
		if nodeToken == "" {
			continue
		}
		if _, ok := seen[nodeToken]; ok {
			continue
		}
		seen[nodeToken] = struct{}{}
		unique = append(unique, nodeToken)
	}

	urls := make(map[string]string, len(unique))
	var batchErrors []error
	for start := 0; start < len(unique); start += wikiCitationMetaBatchMaxRequests {
		end := min(start+wikiCitationMetaBatchMaxRequests, len(unique))
		batch := unique[start:end]
		requestDocs := make([]map[string]interface{}, 0, len(batch))
		for _, nodeToken := range batch {
			requestDocs = append(requestDocs, map[string]interface{}{
				"doc_token": nodeToken,
				"doc_type":  "wiki",
			})
		}

		data, err := runtime.CallAPITyped(
			"POST",
			"/open-apis/drive/v1/metas/batch_query",
			nil,
			map[string]interface{}{
				"request_docs": requestDocs,
				"with_url":     true,
			},
		)
		if err != nil {
			batchErrors = append(batchErrors, err)
			continue
		}

		fallbackToken := ""
		if len(batch) == 1 {
			fallbackToken = batch[0]
		}
		for _, item := range common.GetSlice(data, "metas") {
			raw, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			meta := projectWikiCitationMeta(raw, fallbackToken)
			if meta.NodeToken != "" && meta.URL != "" {
				urls[meta.NodeToken] = meta.URL
			}
		}
	}

	switch len(batchErrors) {
	case 0:
		return urls, nil
	case 1:
		return urls, batchErrors[0]
	default:
		return urls, errors.Join(batchErrors...)
	}
}

func projectWikiCitationMeta(raw map[string]interface{}, fallbackToken string) wikiCitationMeta {
	requestInfo := common.GetMap(raw, "request_doc_info")
	nodeToken := strings.TrimSpace(common.GetString(requestInfo, "doc_token"))
	if nodeToken == "" {
		nodeToken = fallbackToken
	}
	return wikiCitationMeta{
		NodeToken: nodeToken,
		URL:       strings.TrimSpace(common.GetString(raw, "url")),
	}
}

// wikiOutputWithCitationURLs enriches the in-memory payload with tenant-native
// URLs returned by Drive Meta. It is called from Execute, never from a citation
// builder; Build remains a pure projection as required by the citation
// framework. Lookup failures are additive: keep any partial URLs, warn on
// stderr, and let the framework drop entries whose URL remains empty.
func wikiOutputWithCitationURLs(runtime *common.RuntimeContext, data any, nodeTokens []string) any {
	urls, err := fetchWikiCitationURLs(runtime, nodeTokens)
	if err != nil {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: %s: citation URL lookup failed: %v\n", runtime.Cmd.CommandPath(), err)
	}
	return wikiCitationPayload{Data: data, URLs: urls}
}

func appendWikiCitationDryRun(runtime *common.RuntimeContext, dry *common.DryRunAPI, nodeToken string) *common.DryRunAPI {
	if !wikiCitationEnvelopeRequested(runtime) {
		return dry
	}
	return dry.
		POST("/open-apis/drive/v1/metas/batch_query").
		Desc("Resolve tenant-native citation URLs in batches of up to 200 Wiki node tokens").
		Body(map[string]interface{}{
			"request_docs": []map[string]interface{}{{
				"doc_token": nodeToken,
				"doc_type":  "wiki",
			}},
			"with_url": true,
		})
}
