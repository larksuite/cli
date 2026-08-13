// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/runtimeplan"
)

func NewCmdEvents(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Consume and manage real-time events",
		Long:  `Unified event consumption system. Use 'event consume <EventKey>' to start consuming events.`,
		// Without SilenceUsage, RunE errors print the full flag help banner.
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			// This hook shadows root's PersistentPreRun, so preserve the matched
			// command for structured error and declared-scope hints.
			f.CurrentCommand = cmd
			return f.RequireCommandRuntimeCapabilities(cmd.Context(), cmd)
		},
	}
	cmdutil.SetRuntimeCapabilities(cmd, runtimeplan.CapabilityRealtimeEvents)

	snap := compileCatalog()
	consume := NewCmdConsume(f, snap)
	bus := NewCmdBus(f, snap)
	list := NewCmdList(f, snap)
	schema := NewCmdSchema(f, snap)
	status := NewCmdStatus(f)
	stop := NewCmdStop(f)
	for _, local := range []*cobra.Command{list, schema, status, stop} {
		cmdutil.SetRuntimeCapabilities(local)
	}
	cmd.AddCommand(consume)
	cmd.AddCommand(list)
	cmd.AddCommand(schema)
	cmd.AddCommand(status)
	cmd.AddCommand(stop)
	cmd.AddCommand(bus)

	return cmd
}
