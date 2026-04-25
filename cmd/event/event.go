// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

func NewCmdEvents(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Consume and manage real-time events",
		Long:  `Unified event consumption system. Use 'event consume <EventKey>' to start consuming events.`,
		// Without SilenceUsage, RunE errors print the full flag help banner.
		SilenceUsage: true,
	}

	cmd.AddCommand(NewCmdConsume(f))
	cmd.AddCommand(NewCmdList(f))
	cmd.AddCommand(NewCmdSchema(f))
	cmd.AddCommand(NewCmdStatus(f))
	cmd.AddCommand(NewCmdStop(f))
	cmd.AddCommand(NewCmdBus(f))

	return cmd
}
