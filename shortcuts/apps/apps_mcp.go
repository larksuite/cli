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

// AppsMCPGet retrieves a runtime connection. The upstream GET is get-or-create,
// so this command is deliberately classified as a write.
var AppsMCPGet = common.Shortcut{
	Service:     appsService,
	Command:     "+mcp-get",
	Tips:        []string{"Example: lark-cli apps +mcp-get --app-id <app_id> --as user"},
	Description: "Get an app's runtime MCP connection (creates a credential if missing; secrets hidden by default)",
	Risk:        "write",
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Scopes:      []string{"spark:app:read"},
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "include-secret", Type: "bool", Desc: "include the complete connection configuration with credentials in stdout"},
	},
	Validate: validateMCPAppID,
	DryRun: func(ctx context.Context, r *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(mcpURL(r, "connection")).Desc("Get runtime MCP connection; creates a credential if missing")
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
		config, err := mcpConnectionOutput(response.ConfigJSON, r.Bool("include-secret"))
		if err != nil {
			return err
		}
		out := struct {
			Config   json.RawMessage `json:"config"`
			Redacted bool            `json:"redacted"`
		}{config, !r.Bool("include-secret")}
		if r.Bool("include-secret") {
			warnMCPSecret(r)
		}
		r.OutFormat(out, nil, func(w io.Writer) { fmt.Fprintln(w, string(config)) })
		return nil
	},
}

// AppsMCPKeyCreate gets or creates the single runtime credential of an app.
// It does not rotate an existing credential or manage a list of keys.
var AppsMCPKeyCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+mcp-key-create",
	Tips:        []string{"Example: lark-cli apps +mcp-key-create --app-id <app_id> --as user"},
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
	if strings.TrimSpace(r.Str("app-id")) == "" {
		return appsValidationParamError("--app-id", "--app-id is required")
	}
	return nil
}

func mcpURL(r *common.RuntimeContext, resource string) string {
	return fmt.Sprintf(mcpAppPath, validate.EncodePathSegment(r.Str("app-id"))) + resource
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

// Default output is a display-only allowlist, not a runnable config. Redact all
// headers and omit unknown fields (e.g. env/args), URL queries and userinfo.
// Explicit secret output preserves the complete upstream JSON, including future
// client options. The CLI never executes it or writes it to a client config.
func mcpConnectionOutput(raw string, includeSecret bool) (json.RawMessage, error) {
	var config struct {
		Servers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers,omitempty"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid MCP connection JSON").WithCause(err)
	}
	if len(config.Servers) == 0 {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "MCP connection has no mcpServers")
	}
	for name, server := range config.Servers {
		u, err := url.Parse(server.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || name == "" {
			return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "MCP connection requires named HTTPS servers")
		}
		u.User = nil
		u.RawQuery = ""
		u.ForceQuery = false
		u.Fragment = ""
		u.RawFragment = ""
		server.URL = u.String()
		for key := range server.Headers {
			server.Headers[key] = "[REDACTED]"
		}
		config.Servers[name] = server
	}
	if includeSecret {
		return json.RawMessage(raw), nil
	}
	result, err := json.Marshal(config)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot encode MCP connection").WithCause(err)
	}
	return result, nil
}

func warnMCPSecret(r *common.RuntimeContext) {
	fmt.Fprintln(r.IO().ErrOut, "warning: output contains MCP credentials; do not share it or commit it. lark-cli does not store these credentials.")
}
