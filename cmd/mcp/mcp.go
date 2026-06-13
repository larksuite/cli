// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package mcp implements a general Model Context Protocol (MCP) client:
// `lark-cli mcp list` and `lark-cli mcp call` talk to ANY MCP server over
// JSON-RPC 2.0 (Streamable HTTP). Unlike the Lark-managed MCP path used by
// shortcuts, these commands take an arbitrary `--url` and send no Lark-specific
// auth — auth, if any, is supplied via repeatable `--header "Key: Value"`.
package mcp

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// NewCmdMCP builds the `mcp` command group.
func NewCmdMCP(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Call any MCP server over JSON-RPC (tools/list, tools/call)",
		Long: `Interact with any Model Context Protocol (MCP) server over JSON-RPC 2.0.

These commands target an arbitrary server via --url (https only) and send no
Lark credentials; pass any auth the server needs with repeatable
--header "Authorization: Bearer <token>". The raw JSON-RPC result is returned
under "data" so it can be filtered with --jq downstream.`,
	}
	// No Lark login is required to call an external MCP server.
	cmdutil.DisableAuthCheck(cmd)
	cmd.AddCommand(NewCmdMCPList(f))
	cmd.AddCommand(NewCmdMCPCall(f))
	return cmd
}
