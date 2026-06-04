// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// enforceLoginHolderGate is the seam both the immediate-login path
// (login.go:367) and the --no-wait/--device-code poll path (login.go:455)
// share for the holder-mismatch policy. The verifyHolder unit tests pin the
// decision matrix in pure form (input tuple → output tuple), and the
// integration tests under login_run_*_test.go drive the whole authLoginRun
// happy path. Neither covers the wrapper itself: that resolveLoginHolder is
// fed the right factory fields, that abortErr short-circuits before the
// stderr write, and that the soft-warning branch writes the Message exactly
// once to the io.Writer the caller passes (not to f.IOStreams.ErrOut, which
// the production callers do thread in but tests should be able to swap).
//
// These three cases pin that wrapper contract directly, so a refactor that
// e.g. moves the stderr write inside verifyHolder, or starts writing to
// f.IOStreams.ErrOut even when an explicit errOut was passed, fails here
// instead of slipping through to a JSON-mode regression.
func TestEnforceLoginHolderGate_CleanMatch_NilNilNoStderr(t *testing.T) {
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:        "default",
				AppId:       "cli_test",
				CurrentUser: "ou_alice",
				Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
			},
		},
	})

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{ProfileName: "default", AppID: "cli_test"})
	// No --user / env override; the wrapper falls through to AppConfig.CurrentUser
	// and the fresh authorization echoes the same open_id, so this is a clean match.
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	var errOut bytes.Buffer
	warning, err := enforceLoginHolderGate(f, "default", "ou_alice", "Alice", &errOut)
	if err != nil {
		t.Fatalf("clean match must not return abortErr; got %v", err)
	}
	if warning != nil {
		t.Errorf("clean match must not return a warning; got %#v", warning)
	}
	if errOut.Len() != 0 {
		t.Errorf("clean match must not write to errOut; got %q", errOut.String())
	}
}

// Flag-source mismatch — the operator typed `--user ou_alice` but the
// device authorized as ou_bob. The wrapper must surface verifyHolder's
// hard abort (a *errs.ConfigError with SubtypeInvalidArgument) and MUST
// NOT write the soft-advisory message to errOut: there is no advisory in
// this branch — verifyHolder returns (nil, abortErr) — and a stray
// stderr write would corrupt the dispatcher's clean error-render path.
func TestEnforceLoginHolderGate_FlagMismatch_AbortNoStderr(t *testing.T) {
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:  "default",
				AppId: "cli_test",
				// CurrentUser intentionally empty so the flag-source path is
				// the ONLY holder selector; otherwise resolveLoginHolder's
				// invocation-priority rule masks the test.
				Users: []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
			},
		},
	})

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{ProfileName: "default", AppID: "cli_test"})
	f.Invocation = cmdutil.InvocationContext{
		Profile:    "default",
		UserOpenId: "ou_alice",
		UserSource: "flag",
	}

	var errOut bytes.Buffer
	warning, err := enforceLoginHolderGate(f, "default", "ou_bob", "Bob", &errOut)
	if err == nil {
		t.Fatal("flag-source mismatch must return abortErr")
	}
	if warning != nil {
		t.Errorf("flag-source mismatch must NOT return a warning; got %#v", warning)
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected *errs.ConfigError, got %T: %v", err, err)
	}
	if cfgErr.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", cfgErr.Subtype)
	}
	// The whole point of this case: an abortErr branch must NOT also write
	// the soft Message to stderr — verifyHolder's contract is that warning
	// is nil whenever abortErr is non-nil, and the wrapper's contract is
	// that it only writes when warning != nil.
	if errOut.Len() != 0 {
		t.Errorf("abort branch must not write to errOut (warning is nil); got %q", errOut.String())
	}
}

// Implied-holder mismatch — the operator did not pass --user / env, but
// AppConfig.CurrentUser names ou_alice while the device authorized as
// ou_bob. The wrapper must let the login proceed (abortErr nil) and emit
// warning.Message to errOut exactly once. This is the seam the JSON-mode
// path piggybacks on: by the time writeLoginSuccess runs, the stderr line
// has already been written, and the JSON `holder_mismatch_warning` field
// surfaces the same warning structurally — so the wrapper must not write
// the message twice (which would double the stderr line under JSON mode
// when stderr is redirected to a tee), and must not skip the write
// (which would deny operators tailing 2>&1 the legacy advisory).
func TestEnforceLoginHolderGate_ImpliedMismatch_WarningWritesOnce(t *testing.T) {
	stubLoginConfigStep8(t, &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:        "default",
				AppId:       "cli_test",
				CurrentUser: "ou_alice", // implied holder — soft branch
				Users:       []core.AppUser{{UserOpenId: "ou_alice", UserName: "Alice"}},
			},
		},
	})

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{ProfileName: "default", AppID: "cli_test"})
	// No --user / env: invocation context has empty UserOpenId and empty
	// UserSource, so resolveLoginHolder falls through to AppConfig.CurrentUser
	// and verifyHolder takes the implied-holder soft branch.
	f.Invocation = cmdutil.InvocationContext{Profile: "default"}

	var errOut bytes.Buffer
	warning, err := enforceLoginHolderGate(f, "default", "ou_bob", "Bob", &errOut)
	if err != nil {
		t.Fatalf("implied-holder mismatch must NOT abort; got %v", err)
	}
	if warning == nil {
		t.Fatal("implied-holder mismatch must return a non-nil warning")
	}

	// Typed-field round-trip: the wrapper hands these straight through from
	// resolveLoginHolder + the fresh-authorization fields, so a refactor
	// that swapped holder/fresh slots (e.g. inverted equality direction in
	// verifyHolder) would corrupt the JSON consumer payload. Pin both
	// halves so a swap is caught at the wrapper, not at the JSON envelope.
	if warning.HolderOpenId != "ou_alice" || warning.HolderUserName != "Alice" {
		t.Errorf("holder slots wrong: openId=%q name=%q", warning.HolderOpenId, warning.HolderUserName)
	}
	if warning.FreshOpenId != "ou_bob" || warning.FreshUserName != "Bob" {
		t.Errorf("fresh slots wrong: openId=%q name=%q", warning.FreshOpenId, warning.FreshUserName)
	}

	// errOut received exactly the warning Message followed by a single
	// trailing newline (Fprintln). The "exactly once" is the regression
	// guard — the wrapper used to be open-coded at both call sites, and
	// re-introducing that pattern (or routing the soft branch through both
	// the wrapper AND a downstream errOut write inside writeLoginSuccess)
	// would double the line.
	got := errOut.String()
	want := warning.Message + "\n"
	if got != want {
		t.Errorf("errOut content drift\n got: %q\nwant: %q", got, want)
	}
	// Defensive — strings.Count for the brand prefix catches the case where
	// some future Fprintln'd context line shares the prefix and lands here.
	if n := strings.Count(got, "[lark-cli]"); n != 1 {
		t.Errorf("errOut should contain `[lark-cli]` exactly once; got %d:\n%s", n, got)
	}
}
