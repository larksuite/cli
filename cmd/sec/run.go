// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	intsec "github.com/larksuite/cli/internal/sec"
)

// RunOptions holds inputs for `lark-cli sec run`.
type RunOptions struct {
	Factory   *cmdutil.Factory
	ProxyPort int
	// AutoInstall runs `sec install` first when no binary is recorded.
	AutoInstall bool
}

// NewCmdSecRun starts lark-sec-cli as a user-level system service so it
// persists across logins and gets restarted by the OS supervisor if it
// crashes. Under the hood it shells out to `lark-sec-cli service enable`,
// which is the recommended startup path per the lark-sec-cli manual:
//
//   - macOS  → user-level launchd plist with KeepAlive=true
//   - Linux  → user systemd unit with Restart=always
//   - Windows → registry autostart + a VBS watchdog loop
//
// Switching to this from a detached `exec.Command(... Setsid:true)` spawn
// fixes two latent issues at once: (1) daemon logs survive past lark-cli
// exit because the service supervisor — not our terminated pipes — owns
// the daemon's stdout, and (2) the daemon's own self-upgrade module can
// now fire (it gates on running-under-supervisor).
func NewCmdSecRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{Factory: f, AutoInstall: true}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Enable lark-sec-cli as a user system service (the daemon runs in the background)",
		Long: `Install lark-sec-cli as a user-level system service so the proxy
daemon runs automatically, persists across logins, and is restarted by the
OS if it exits. The daemon writes its own log file (default: under
~/.lark-sec-cli/logs/daemon.log) so logs persist independently of this
command.

After enabling, the daemon writes ~/.lark-cli/sec_config.json itself with
the proxy port and CA path, so subsequent lark-cli runs route through the
sidecar without any further action.

To stop and remove the service: lark-cli sec stop.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return runRun(cmd, opts)
		},
	}
	cmd.Flags().IntVar(&opts.ProxyPort, "proxy-port", 0, "force lark-sec-cli to bind this port (default: dynamic)")
	cmd.Flags().BoolVar(&opts.AutoInstall, "auto-install", true, "bootstrap-install lark-sec-cli first when no binary is recorded")
	return cmd
}

func runRun(cmd *cobra.Command, opts *RunOptions) error {
	ctx := cmd.Context()
	errOut := opts.Factory.IOStreams.ErrOut
	trace := verboseOut(cmd, errOut)

	tracef(trace, "sec run", "constructing installer (lazy credentials)")
	inst, paths, err := installer(opts.Factory)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "%v", err)
	}

	// Make sure we have a binary on disk before asking it to install itself
	// as a service.
	tracef(trace, "sec run", "loading state from %s", paths.StateFile())
	state, err := intsec.LoadState(paths.StateFile())
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "load sec state: %v", err)
	}
	if state == nil {
		tracef(trace, "sec run", "no install on disk (auto-install=%t)", opts.AutoInstall)
		if !opts.AutoInstall {
			return output.ErrWithHint(output.ExitValidation, "sec_not_installed",
				"lark-sec-cli is not installed",
				"Re-run `lark-cli sec run` with --auto-install (default on), or remove --auto-install=false.")
		}
		state, err = inst.Install(ctx, intsec.InstallOptions{Verbose: trace})
		if err != nil {
			return output.Errorf(output.ExitNetwork, "sec_install", "auto-install lark-sec-cli: %v", err)
		}
	} else {
		tracef(trace, "sec run", "existing install: version=%s binary=%s", state.Version, state.BinaryPath)
	}

	args := []string{"service", "enable"}
	if opts.ProxyPort > 0 {
		args = append(args, fmt.Sprintf("--proxy-port=%d", opts.ProxyPort))
	}

	fmt.Fprintf(errOut, "Running: %s %v\n", state.BinaryPath, args)
	tracef(trace, "sec run", "shelling out to %s %v", state.BinaryPath, args)

	c := exec.CommandContext(ctx, state.BinaryPath, args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return output.Errorf(output.ExitInternal, "sec_service_enable",
			"`lark-sec-cli service enable` failed: %v\nstderr: %s", err, stderr.String())
	}
	tracef(trace, "sec run", "service enable returned ok (%d bytes stdout)", stdout.Len())

	// Forward the installer's stdout to the user — it contains the launchd /
	// systemd unit name, the registered executable path, and a confirmation
	// that the supervisor will respawn the daemon on exit. Useful diagnostic
	// output that's better seen than swallowed.
	fmt.Fprint(errOut, stdout.String())
	output.PrintSuccess(errOut,
		"lark-sec-cli enabled as a user system service. Run `lark-cli sec status` to verify, `lark-cli sec stop` to disable.")
	return nil
}
