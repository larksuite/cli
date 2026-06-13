// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mcp

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

type listOptions struct {
	Factory *cmdutil.Factory
	URL     string
	Headers []string
}

// NewCmdMCPList builds `mcp list` — JSON-RPC tools/list against --url.
func NewCmdMCPList(f *cmdutil.Factory) *cobra.Command {
	opts := &listOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the tools an MCP server exposes (tools/list)",
		Example: `  lark-cli mcp list --url https://example.com/mcp
  lark-cli mcp list --url https://example.com/mcp --header "Authorization: Bearer $TOKEN"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.URL, "url", "", "MCP server URL (https://...)")
	cmd.Flags().StringArrayVar(&opts.Headers, "header", nil, "extra request header \"Key: Value\" (repeatable)")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func runList(ctx context.Context, opts *listOptions) error {
	headers, err := parseHeaders(opts.Headers)
	if err != nil {
		return err
	}
	client, err := httpClientFor(ctx, opts.Factory, opts.URL)
	if err != nil {
		return err
	}
	result, err := jsonRPC(ctx, client, opts.URL, "tools/list", map[string]any{}, headers)
	if err != nil {
		return err
	}
	output.PrintJson(opts.Factory.IOStreams.Out, map[string]interface{}{"ok": true, "data": result})
	return nil
}
