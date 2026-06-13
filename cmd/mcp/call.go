// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

type callOptions struct {
	Factory *cmdutil.Factory
	URL     string
	Tool    string
	Args    string
	Headers []string
}

// NewCmdMCPCall builds `mcp call` — JSON-RPC tools/call against --url.
func NewCmdMCPCall(f *cmdutil.Factory) *cobra.Command {
	opts := &callOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "call",
		Short: "Call a tool on an MCP server (tools/call)",
		Example: `  lark-cli mcp call --url https://example.com/mcp --tool search --args '{"query":"hello"}'
  lark-cli mcp call --url https://example.com/mcp --tool get_item --args '{"id":1}' --header "Authorization: Bearer $TOKEN"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCall(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.URL, "url", "", "MCP server URL (https://...)")
	cmd.Flags().StringVar(&opts.Tool, "tool", "", "tool name to call")
	cmd.Flags().StringVar(&opts.Args, "args", "{}", "tool arguments as a JSON object")
	cmd.Flags().StringArrayVar(&opts.Headers, "header", nil, "extra request header \"Key: Value\" (repeatable)")
	_ = cmd.MarkFlagRequired("url")
	_ = cmd.MarkFlagRequired("tool")
	return cmd
}

func runCall(ctx context.Context, opts *callOptions) error {
	raw := strings.TrimSpace(opts.Args)
	if raw == "" {
		raw = "{}"
	}
	var arguments map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
		return output.ErrValidation("invalid --args (must be a JSON object): %v", err)
	}

	headers, err := parseHeaders(opts.Headers)
	if err != nil {
		return err
	}
	client, err := httpClientFor(ctx, opts.Factory, opts.URL)
	if err != nil {
		return err
	}
	params := map[string]any{"name": opts.Tool, "arguments": arguments}
	result, err := jsonRPC(ctx, client, opts.URL, "tools/call", params, headers)
	if err != nil {
		return err
	}
	output.PrintJson(opts.Factory.IOStreams.Out, map[string]interface{}{"ok": true, "data": result})
	return nil
}
