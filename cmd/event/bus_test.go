// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// The hidden `event _bus` daemon command must exit with a typed file_io error
// when its log directory cannot be created (the error is only visible in the
// forked process's captured stderr / bus.log).
func TestBusCommandLoggerSetupFailureIsTypedFileIO(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	// Block the events/ root with a regular file so MkdirAll fails.
	if err := os.WriteFile(filepath.Join(dir, "events"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "cli_bus_test", AppSecret: "secret", Brand: core.BrandFeishu,
	})
	cmd := NewCmdBus(f)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected logger setup error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeFileIO {
		t.Errorf("problem = %s/%s, want %s/%s", p.Category, p.Subtype,
			errs.CategoryInternal, errs.SubtypeFileIO)
	}
}
