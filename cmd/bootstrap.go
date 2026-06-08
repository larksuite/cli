// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/migrate"
	"github.com/spf13/pflag"
)

// migrateOnce gates MaybeMigrate to a single run per process: cobra
// re-parses global flags on completion/help paths, and we must not
// migrate twice. Errors are swallowed so a failed migration can never
// block command dispatch; the migrator retries on the next invocation.
var migrateOnce sync.Once

// runMigrationOnce is a var so tests can replace it with a no-op.
var runMigrationOnce = func() {
	migrateOnce.Do(func() {
		root := larkauth.NewLocalRoot(core.GetConfigDir())
		_ = migrate.MaybeMigrate(root, os.Stderr)
	})
}

// BootstrapInvocationContext extracts global invocation options before
// the real command tree is built, so provider-backed config resolution sees
// the correct profile from the start.
//
// User-selection precedence:
//  1. --user flag (trimmed). An explicit empty value is a hard error,
//     never a silent fallthrough to env.
//  2. LARKSUITE_CLI_OPEN_ID (trimmed), only when --user was not passed.
//  3. Empty; downstream core.ResolveConfigFromMulti walks
//     AppConfig.CurrentUser then Users[0].
//
// The env var is read here ONLY — credential and resolver layers stay
// env-agnostic (enforced by canary tests).
func BootstrapInvocationContext(args []string) (cmdutil.InvocationContext, error) {
	var globals GlobalOptions

	fs := pflag.NewFlagSet("bootstrap", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	fs.SetInterspersed(true)
	fs.SetOutput(io.Discard)
	RegisterGlobalFlags(fs, &globals)

	if err := fs.Parse(args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return cmdutil.InvocationContext{}, err
	}

	// Run migration after flag parse (so [WARN] lines are visible) but
	// before buildInternal (so subcommands see the migrated state).
	runMigrationOnce()

	var (
		userOverride string
		userSource   string
	)
	if fs.Changed("user") {
		u := strings.TrimSpace(globals.User)
		if u == "" {
			return cmdutil.InvocationContext{}, errs.NewConfigError(errs.SubtypeInvalidArgument,
				"--user requires a non-empty value").
				WithHint("run `lark-cli auth users list` to see available users")
		}
		userOverride, userSource = u, "flag"
	} else if u := strings.TrimSpace(os.Getenv(envvars.CliOpenID)); u != "" {
		userOverride, userSource = u, "env"
	}

	return cmdutil.InvocationContext{
		Profile:    globals.Profile,
		UserOpenId: userOverride,
		UserSource: userSource,
	}, nil
}
