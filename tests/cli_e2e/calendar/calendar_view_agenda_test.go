// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestCalendar_ViewAgenda tests the workflow of viewing one's calendar agenda.
func TestCalendar_ViewAgenda(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	t.Run("view today agenda", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+agenda"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("view agenda with date range", func(t *testing.T) {
		startDate := time.Now().UTC().Format("2006-01-02")
		endDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"calendar", "+agenda", "--start", startDate, "--end", endDate},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("view agenda with pretty format", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:   []string{"calendar", "+agenda"},
			Format: "pretty",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
	})
}