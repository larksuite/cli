// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event/bus"
	"github.com/larksuite/cli/internal/event/transport"
)

// NewCmdBus creates the hidden `event _bus` daemon subcommand. Forked
// by the consume client; not meant to be called directly. The fork
// command line is built in internal/event/consume/startup.go — keep the
// two in sync when changing the command path.
func NewCmdBus(f *cmdutil.Factory) *cobra.Command {
	var domain string

	cmd := &cobra.Command{
		Use:    "_bus",
		Short:  "Internal event bus daemon (do not call directly)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}

			eventsDir := filepath.Join(core.GetConfigDir(), "events", cfg.AppID)

			logger, err := bus.SetupBusLogger(eventsDir)
			if err != nil {
				return err
			}

			tr := transport.New()
			b := bus.NewBus(cfg.AppID, cfg.AppSecret, domain, tr, logger)

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
			defer signal.Stop(sigCh)
			go func() {
				select {
				case <-sigCh:
					cancel()
				case <-ctx.Done():
				}
			}()

			return b.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "API domain")
	_ = cmd.Flags().MarkHidden("domain")

	return cmd
}
