// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// docsSceneContextKey lets in-process embedders pass a server-owned docs_ai
// scene without exposing it as a user-controlled CLI flag.
const docsSceneContextKey = "lark_cli_docs_scene"

type documentRef struct {
	Kind  string
	Token string
}

func parseDocumentRef(input string) (documentRef, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return documentRef{}, output.ErrValidation("--doc cannot be empty")
	}

	if token, ok := extractDocumentToken(raw, "/wiki/"); ok {
		return documentRef{Kind: "wiki", Token: token}, nil
	}
	if token, ok := extractDocumentToken(raw, "/docx/"); ok {
		return documentRef{Kind: "docx", Token: token}, nil
	}
	if token, ok := extractDocumentToken(raw, "/doc/"); ok {
		return documentRef{Kind: "doc", Token: token}, nil
	}
	if strings.Contains(raw, "://") {
		return documentRef{}, output.ErrValidation("unsupported --doc input %q: use a docx URL/token or a wiki URL that resolves to docx", raw)
	}
	if strings.ContainsAny(raw, "/?#") {
		return documentRef{}, output.ErrValidation("unsupported --doc input %q: use a docx token or a wiki URL", raw)
	}

	return documentRef{Kind: "docx", Token: raw}, nil
}

func extractDocumentToken(raw, marker string) (string, bool) {
	idx := strings.Index(raw, marker)
	if idx < 0 {
		return "", false
	}
	token := raw[idx+len(marker):]
	if end := strings.IndexAny(token, "/?#"); end >= 0 {
		token = token[:end]
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	return token, true
}

// doDocAPI executes an OpenAPI request against the docs_ai endpoints and returns
// the parsed "data" field from the standard Lark response envelope {code, msg, data}.
// Uses the log-id-aware variant so the x-tt-logid header is surfaced in both the
// success payload and error details — doc v2 callers rely on it for support escalations.
func doDocAPI(runtime *common.RuntimeContext, method, apiPath string, body interface{}) (map[string]interface{}, error) {
	return runtime.DoAPIJSONWithLogID(method, apiPath, nil, body)
}

func docsSceneFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	scene, _ := ctx.Value(docsSceneContextKey).(string)
	return strings.TrimSpace(scene)
}

func injectDocsScene(runtime *common.RuntimeContext, body map[string]interface{}) {
	if scene := docsSceneFromContext(runtime.Ctx()); scene != "" {
		body["scene"] = scene
	}
}

func buildDriveRouteExtra(docID string) (string, error) {
	extra, err := json.Marshal(map[string]string{"drive_route_token": docID})
	if err != nil {
		return "", output.Errorf(output.ExitInternal, "internal_error", "failed to marshal upload extra data: %v", err)
	}
	return string(extra), nil
}
