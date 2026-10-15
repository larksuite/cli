// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const mcpAppPath = apiBasePath + "/apps/%s/mcp/"

// mcpURLAuthParam is the name of the query parameter that carries the runtime
// credential in the MCP URL; it is a parameter name, not a credential value.
const mcpURLAuthParam = "x-mcp-token"

// AppsMCPGet retrieves the runtime MCP URL. The upstream GET is get-or-create,
// so this command is deliberately classified as a write.
var AppsMCPGet = common.Shortcut{
	Service:     appsService,
	Command:     "+mcp-get",
	Description: "Get an app's runtime MCP URL with its app token (a developer's call creates the credential if missing; the MCP client must still complete user OAuth to call tools; use --format pretty to print only the URL)",
	Risk:        "write",
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Scopes:      []string{"spark:app:read"},
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
	},
	Validate: validateMCPAppID,
	DryRun: func(ctx context.Context, r *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(mcpURL(r, "connection")).Desc("Get runtime MCP URL; creates a credential if missing")
	},
	Execute: func(ctx context.Context, r *common.RuntimeContext) error {
		data, err := r.CallAPITyped("GET", mcpURL(r, "connection"), nil, nil)
		if err != nil {
			return err
		}
		var response struct {
			ConfigJSON string `json:"config_json"`
		}
		if err := decodeMCPResponse(data, &response); err != nil {
			return err
		}
		// Validate has already rejected blank IDs before Execute.
		appID, _ := requireAppID(r.Str("app-id"))
		serverURL, err := mcpServerURL(response.ConfigJSON, appID)
		if err != nil {
			return err
		}
		out := struct {
			URL string `json:"url"`
		}{serverURL}
		r.OutFormat(out, nil, func(w io.Writer) { fmt.Fprintln(w, serverURL) })
		return nil
	},
}

// AppsMCPKeyCreate gets or creates the single runtime credential of an app.
// It does not rotate an existing credential or manage a list of keys.
var AppsMCPKeyCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+mcp-key-create",
	Description: "Get or create an app's runtime MCP credential (secrets hidden by default)",
	Risk:        "write",
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Scopes:      []string{"spark:app:write"},
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "include-secret", Type: "bool", Desc: "include the credential's plaintext key in stdout"},
	},
	Validate: validateMCPAppID,
	DryRun: func(ctx context.Context, r *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().POST(mcpURL(r, "credential")).Desc("Get or create runtime MCP credential; never rotate")
	},
	Execute: func(ctx context.Context, r *common.RuntimeContext) error {
		data, err := r.CallAPITyped("POST", mcpURL(r, "credential"), nil, nil)
		if err != nil {
			return err
		}
		var response struct {
			CredentialID string `json:"credential_id"`
			PlainKey     string `json:"plain_key"`
		}
		if err := decodeMCPResponse(data, &response); err != nil {
			return err
		}
		if strings.TrimSpace(response.CredentialID) == "" || strings.TrimSpace(response.PlainKey) == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "MCP credential response is missing credential_id or plain_key")
		}
		out := struct {
			CredentialID string `json:"credential_id"`
			PlainKey     string `json:"plain_key,omitempty"`
			Redacted     bool   `json:"redacted"`
		}{CredentialID: response.CredentialID, Redacted: !r.Bool("include-secret")}
		if r.Bool("include-secret") {
			out.PlainKey = response.PlainKey
			warnMCPSecret(r)
		}
		r.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Credential ID: %s\n", out.CredentialID)
			if out.PlainKey != "" {
				fmt.Fprintf(w, "Key: %s\n", out.PlainKey)
			}
		})
		return nil
	},
}

func validateMCPAppID(_ context.Context, r *common.RuntimeContext) error {
	_, err := requireAppID(r.Str("app-id"))
	return err
}

func mcpURL(r *common.RuntimeContext, resource string) string {
	// Validate has already rejected blank IDs before DryRun or Execute.
	appID, _ := requireAppID(r.Str("app-id"))
	return fmt.Sprintf(mcpAppPath, validate.EncodePathSegment(appID)) + resource
}

func decodeMCPResponse(data map[string]interface{}, target interface{}) error {
	raw, err := json.Marshal(data)
	if err == nil {
		err = json.Unmarshal(raw, target)
	}
	if err != nil {
		// Do not render decoder errors: a malformed value may contain credentials.
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid MCP response field types").WithCause(err)
	}
	return nil
}

// mcpServerURL extracts the HTTPS URL of the server keyed by the app ID from
// the upstream connection JSON and requires it to carry a token. The URL
// carries the credential, so decoder errors are never rendered.
func mcpServerURL(raw, appID string) (string, error) {
	var config struct {
		Servers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid MCP connection JSON").WithCause(err)
	}
	server, ok := config.Servers[appID]
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "MCP connection has no server for the requested app")
	}
	u, err := url.Parse(server.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "MCP connection requires an HTTPS server URL")
	}
	// The URL is the only thing handed to the client, so a URL without its
	// token would be reported as success yet unusable.
	if strings.TrimSpace(u.Query().Get(mcpURLAuthParam)) == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "MCP connection URL is missing the "+mcpURLAuthParam+" credential")
	}
	return server.URL, nil
}

func warnMCPSecret(r *common.RuntimeContext) {
	fmt.Fprintln(r.IO().ErrOut, "warning: output contains MCP credentials; do not share it or commit it. lark-cli does not store these credentials.")
}
