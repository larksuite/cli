// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
)

// SetAppSecretOptions holds all inputs for config set-app-secret.
type SetAppSecretOptions struct {
	Factory        *cmdutil.Factory
	AppSecretStdin bool
	Yes            bool
}

// NewCmdConfigSetAppSecret creates the config set-app-secret subcommand.
func NewCmdConfigSetAppSecret(f *cmdutil.Factory) *cobra.Command {
	opts := &SetAppSecretOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "set-app-secret",
		Short: "Rotate a profile's stored app secret (verified before saving)",
		Long: `Rotate a profile's app secret after you reset it on the Lark/Feishu open platform.
The new secret is verified against Lark before anything is saved; only the target
profile changes — other profiles and the active selection stay untouched.

Run it in a terminal with no flags for an interactive prompt (pick the profile,
confirm, then type the secret hidden). Pipe the secret with --app-secret-stdin
(and --yes to apply) for scripts and AI agents.

Targets the active profile; use the global --profile <name|app_id> to pick another.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return setAppSecretRun(f, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.AppSecretStdin, "app-secret-stdin", false, "Read the new secret from stdin (required with --yes).")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "Apply; without it, only preview the target and exit 10.")
	// Do NOT call MarkFlagRequired("app-secret-stdin") — preview path (no --yes)
	// must work without it; the "required" is enforced inside setAppSecretRun on
	// the apply path only.
	cmdutil.SetRisk(cmd, "high-risk-write")

	return cmd
}

// setAppSecretRun dispatches between the interactive (human at a TTY) and the
// non-interactive (agent / piped / explicit-flag) paths. Both share the same
// verify-before-write and storage logic; only the way the target and the new
// secret are acquired differs.
func setAppSecretRun(f *cmdutil.Factory, opts *SetAppSecretOptions) error {
	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return err
	}
	profileOverride := f.Invocation.Profile

	// Interactive mode: a human at a terminal who passed neither --app-secret-stdin
	// nor --yes. Mirrors config bind's IsTUI gate. Any machine-facing flag, or a
	// non-terminal stdout, takes the non-interactive path so agents/scripts keep
	// the structured exit-10 + stdin protocol unchanged.
	if f.IOStreams.IsTerminal && !opts.AppSecretStdin && !opts.Yes {
		return setAppSecretInteractive(f, multi, profileOverride)
	}
	return setAppSecretNonInteractive(f, opts, multi, profileOverride)
}

// ── Non-interactive (agent / piped / explicit-flag) path ─────────────────────

// setAppSecretNonInteractive implements the structured protocol: confirm gate
// (no --yes → preview target + exit 10), require --app-secret-stdin, read the
// secret from stdin, verify, write, emit envelope.
func setAppSecretNonInteractive(f *cmdutil.Factory, opts *SetAppSecretOptions, multi *core.MultiAppConfig, profileOverride string) error {
	app := multi.CurrentAppConfig(profileOverride)
	if app == nil {
		if profileOverride != "" {
			return errs.NewConfigError(errs.SubtypeNotConfigured,
				"profile %q not found", profileOverride).
				WithHint("available profiles: %s", joinProfileNames(multi.ProfileNames()))
		}
		return core.NoActiveProfileError()
	}
	target := buildTarget(multi, app)

	// Confirm gate — MUST happen before any stdin read.
	if !opts.Yes {
		// Follow the framework confirmation convention (internal/cmdutil/confirm.go):
		// `action` is the operation identifier, and the hint tells the caller what to
		// append to THEIR OWN invocation — never a pre-built "lark-cli …" string.
		// A pre-built command hardcodes the binary name and trips shell-quoting /
		// wrong-binary pitfalls (e.g. an older installed `lark-cli` without this
		// subcommand). We add --profile <app_id> to the guidance so the apply pins
		// the previewed target and cannot drift to a different active profile.
		msg := fmt.Sprintf(
			"app secret for profile %q (%s) will be rotated; confirm the target, then re-run with --profile %s --yes",
			app.ProfileName(), app.AppId, app.AppId,
		)
		hint := fmt.Sprintf("add --profile %s --yes to confirm (pins the target shown above; pipe the new secret via stdin)", app.AppId)
		return errs.NewConfirmationRequiredError(
			errs.RiskHighRiskWrite,
			"config set-app-secret",
			"%s", msg,
		).WithHint("%s", hint).WithTarget(target)
	}

	// Apply path: require --app-secret-stdin.
	if !opts.AppSecretStdin {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "app secret must be provided via stdin").
			WithHint("use --app-secret-stdin and pipe the secret").
			WithParam("--app-secret-stdin")
	}

	// Read and validate the new secret from stdin.
	scanner := bufio.NewScanner(f.IOStreams.In)
	if !scanner.Scan() {
		if scanErr := scanner.Err(); scanErr != nil {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "failed to read secret from stdin: %v", scanErr).
				WithCause(scanErr).
				WithParam("--app-secret-stdin")
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "stdin is empty, expected app secret").
			WithHint("pipe the app secret to stdin").
			WithParam("--app-secret-stdin")
	}
	newSecret := strings.TrimSpace(scanner.Text())
	if newSecret == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "app secret read from stdin is empty").
			WithHint("pipe a non-empty app secret to stdin").
			WithParam("--app-secret-stdin")
	}

	// Verify, then write. verifyNewSecret returns typed errors (rendered as the
	// structured envelope) carrying agent-facing retry hints.
	if err := verifyNewSecret(f, app, target, newSecret); err != nil {
		return err
	}
	migrated, err := storeNewSecret(multi, app, newSecret, f)
	if err != nil {
		return err
	}
	return emitSuccess(f, target, migrated)
}

// ── Interactive (human at a TTY) path ────────────────────────────────────────

// secretPrompter supplies the three interactive steps (profile pick, confirm,
// secret entry). The default implementation is backed by huh; tests inject fakes
// to drive runInteractive's orchestration without a real terminal.
type secretPrompter struct {
	selectProfile func(multi *core.MultiAppConfig, profileOverride string) (*core.AppConfig, error)
	confirm       func(target *errs.ErrTarget) (bool, error)
	readInput     func() (string, error)
}

// defaultSecretPrompter wires the huh-backed interactive steps.
func defaultSecretPrompter() secretPrompter {
	return secretPrompter{
		selectProfile: selectTargetProfile,
		confirm:       confirmRotate,
		readInput:     promptHiddenInput,
	}
}

// setAppSecretInteractive walks a human through profile selection, an explicit
// confirmation, and a hidden secret entry, then runs the same verify-before-write
// and storage logic.
func setAppSecretInteractive(f *cmdutil.Factory, multi *core.MultiAppConfig, profileOverride string) error {
	return runInteractive(f, multi, profileOverride, defaultSecretPrompter())
}

// runInteractive orchestrates the interactive flow against the supplied prompter,
// then runs the same verify-before-write / storage logic as the agent path.
// Failures are rendered as readable lines (not the agent JSON envelope).
func runInteractive(f *cmdutil.Factory, multi *core.MultiAppConfig, profileOverride string, p secretPrompter) error {
	app, err := p.selectProfile(multi, profileOverride)
	if err != nil {
		return err
	}
	target := buildTarget(multi, app)

	confirmed, err := p.confirm(target)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(f.IOStreams.ErrOut, "cancelled — nothing was changed")
		return nil // explicit decline is a clean no-op (exit 0)
	}

	newSecret, err := p.readInput()
	if err != nil {
		return err
	}

	if err := verifyNewSecret(f, app, target, newSecret); err != nil {
		// Render a readable line for humans instead of the agent JSON envelope.
		code := output.ExitCodeOf(err)
		humanMsg := err.Error()
		if pr, ok := errs.ProblemOf(err); ok {
			humanMsg = pr.Message
		}
		fmt.Fprintf(f.IOStreams.ErrOut, "✗ %s\n", humanMsg)
		fmt.Fprintln(f.IOStreams.ErrOut, "  run the command again to retry")
		return output.ErrBare(code)
	}
	migrated, err := storeNewSecret(multi, app, newSecret, f)
	if err != nil {
		return err
	}
	return emitSuccess(f, target, migrated)
}

// selectTargetProfile resolves the profile to rotate. With --profile it uses
// that one; otherwise with a single profile it uses it; with several it shows
// a picker (active pre-selected).
func selectTargetProfile(multi *core.MultiAppConfig, profileOverride string) (*core.AppConfig, error) {
	if profileOverride != "" {
		app := multi.CurrentAppConfig(profileOverride)
		if app == nil {
			return nil, errs.NewConfigError(errs.SubtypeNotConfigured,
				"profile %q not found", profileOverride).
				WithHint("available profiles: %s", joinProfileNames(multi.ProfileNames()))
		}
		return app, nil
	}
	if len(multi.Apps) == 0 {
		return nil, core.NoActiveProfileError()
	}
	if len(multi.Apps) == 1 {
		return &multi.Apps[0], nil
	}

	options, selected := profileSelectOptions(multi)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Select the profile whose app secret to rotate").
				Options(options...).
				Value(&selected),
		),
	).WithTheme(cmdutil.ThemeFeishu())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, output.ErrBare(1)
		}
		return nil, err
	}
	return &multi.Apps[selected], nil
}

// profileSelectOptions builds the huh picker options for multiple profiles,
// labelling and pre-selecting the active one. Pure logic, unit-tested.
func profileSelectOptions(multi *core.MultiAppConfig) ([]huh.Option[int], int) {
	activeApp := multi.CurrentAppConfig("")
	options := make([]huh.Option[int], 0, len(multi.Apps))
	selected := 0
	for i := range multi.Apps {
		a := &multi.Apps[i]
		label := fmt.Sprintf("%s (%s)", a.ProfileName(), a.AppId)
		if activeApp != nil && activeApp.ProfileName() == a.ProfileName() {
			label += " [active]"
			selected = i
		}
		options = append(options, huh.NewOption(label, i))
	}
	return options, selected
}

// confirmRotate shows a y/N confirmation for the resolved target.
func confirmRotate(target *errs.ErrTarget) (bool, error) {
	activeSeg := ""
	if target.IsActive {
		activeSeg = " [active]"
	}
	confirmed := false
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Rotate the app secret for profile %q (%s)%s?", target.Profile, target.AppID, activeSeg)).
				Description("The new secret is verified against Lark before anything is saved.").
				Affirmative("Yes, rotate").
				Negative("Cancel").
				Value(&confirmed),
		),
	).WithTheme(cmdutil.ThemeFeishu())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, output.ErrBare(1)
		}
		return false, err
	}
	return confirmed, nil
}

// promptHiddenInput reads the new secret with a hidden input (matches
// config init's existing app-secret prompt style).
func promptHiddenInput() (string, error) {
	var secret string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter the new app secret").
				EchoMode(huh.EchoModePassword).
				Validate(validateAppSecret).
				Value(&secret),
		),
	).WithTheme(cmdutil.ThemeFeishu())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", output.ErrBare(1)
		}
		return "", err
	}
	return strings.TrimSpace(secret), nil
}

// validateAppSecret is the huh input validator: the new secret must be non-empty.
// Pure logic, unit-tested.
func validateAppSecret(s string) error {
	if strings.TrimSpace(s) == "" {
		//nolint:forbidigo // huh inline form-validation message shown in the TUI, not a final CLI error
		return fmt.Errorf("app secret must not be empty")
	}
	return nil
}

// ── Shared verify / write / output ───────────────────────────────────────────

// buildTarget assembles the structured target identity for the resolved profile.
// IsActive is true when the resolved profile equals the default active one.
func buildTarget(multi *core.MultiAppConfig, app *core.AppConfig) *errs.ErrTarget {
	activeApp := multi.CurrentAppConfig("")
	return &errs.ErrTarget{
		Profile:  app.ProfileName(),
		AppID:    app.AppId,
		IsActive: activeApp != nil && activeApp.ProfileName() == app.ProfileName(),
	}
}

// verifyNewSecret validates newSecret against Lark before any write, using the
// same FetchTAT + errs.IsTyped discriminator as cmd/config/init_probe.go. A typed
// error is a deterministic credential rejection (exit 3); an untyped error is
// transient/transport (exit 4). Neither path writes anything.
func verifyNewSecret(f *cmdutil.Factory, app *core.AppConfig, target *errs.ErrTarget, newSecret string) error {
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer verifyCancel()
	httpClient, err := f.HttpClient()
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "could not initialise HTTP client for secret verification, nothing was changed, please retry").
			WithHint("the target is already confirmed — re-run with --profile %s --yes (no need to preview again)", app.AppId).
			WithCause(err).
			WithTarget(target)
	}
	if _, err := credential.FetchTAT(verifyCtx, httpClient, app.Brand, app.AppId, newSecret); err != nil {
		if errs.IsTyped(err) {
			// Deterministic credential rejection (invalid_client / unauthorized_client).
			return errs.NewConfigError(errs.SubtypeInvalidClient, "new app secret is invalid, nothing was changed").
				WithHint("the target is already confirmed — provide a valid secret and re-run with --profile %s --yes (no need to preview again)", app.AppId).
				WithCause(err).
				WithTarget(target)
		}
		// Transient / transport / timeout — surface as NetworkError so the caller
		// knows to retry rather than treat it as a bad credential.
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "could not verify the new secret (transient error), nothing was changed, please retry").
			WithHint("the target is already confirmed — re-run with --profile %s --yes (no need to preview again)", app.AppId).
			WithCause(err).
			WithTarget(target)
	}
	return nil
}

// storeNewSecret writes newSecret via core.ForStorage (the single canonical
// secret-storage primitive, identical to config init / config bind) and updates
// config.json only when migrating a plaintext/file source to a keychain ref.
// Returns migrated=true only in that migration case.
func storeNewSecret(multi *core.MultiAppConfig, app *core.AppConfig, newSecret string, f *cmdutil.Factory) (bool, error) {
	stored, err := core.ForStorage(app.AppId, core.PlainSecret(newSecret), f.Keychain)
	if err != nil {
		return false, errs.NewInternalError(errs.SubtypeStorage, "%v", err).WithCause(err)
	}

	// If the existing secret is already a keychain ref, ForStorage overwrote the
	// keychain entry in-place — the ref is unchanged, so we must NOT rewrite
	// config.json (byte-level stability). For plain/file sources, migrate the ref.
	idx := multi.FindAppIndex(app.AppId)
	orig := multi.Apps[idx].AppSecret
	if orig.IsSecretRef() && orig.Ref.Source == "keychain" {
		return false, nil
	}
	// plain/file source: update only this profile's AppSecret field; all other
	// profiles and all other fields of this profile are left completely untouched.
	multi.Apps[idx].AppSecret = stored
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return false, errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}
	return true, nil
}

// emitSuccess writes the result: a JSON envelope on stdout for non-terminal
// (piped / agent / script) callers, a pretty line for a human terminal.
func emitSuccess(f *cmdutil.Factory, target *errs.ErrTarget, migrated bool) error {
	if !f.IOStreams.IsTerminal {
		envelope := map[string]any{
			"ok":       true,
			"identity": "bot",
			"data": map[string]any{
				"profile":   target.Profile,
				"app_id":    target.AppID,
				"is_active": target.IsActive,
				"verified":  true,
				"migrated":  migrated,
			},
		}
		resultJSON, _ := json.Marshal(envelope)
		fmt.Fprintln(f.IOStreams.Out, string(resultJSON))
		return nil
	}
	// Pretty line: ✓ app secret updated for profile "cursor" (cli_xxxxx) [active] — verified
	activeSegment := ""
	if target.IsActive {
		activeSegment = " [active]"
	}
	fmt.Fprintf(f.IOStreams.Out,
		"✓ app secret updated for profile %q (%s)%s — verified\n",
		target.Profile, target.AppID, activeSegment,
	)
	return nil
}

// joinProfileNames joins profile names for a hint message.
func joinProfileNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	result := ""
	for i, n := range names {
		if i > 0 {
			result += ", "
		}
		result += n
	}
	return result
}
