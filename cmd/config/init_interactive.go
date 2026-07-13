// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/charmbracelet/huh"
	"github.com/larksuite/cli/internal/build"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/transport"
)

// configInitResult holds the result of the interactive config init flow.
type configInitResult struct {
	Mode      string // "create" or "existing"
	Brand     core.LarkBrand
	AppID     string
	AppSecret string
}

// runInteractiveConfigInit shows an interactive TUI for config init.
func runInteractiveConfigInit(ctx context.Context, f *cmdutil.Factory, msg *initMsg) (*configInitResult, error) {
	// Phase 1: Choose mode
	var mode string
	form1 := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(msg.SelectAction).
				Options(
					huh.NewOption(msg.CreateNewApp, "create"),
					huh.NewOption(msg.ConfigExistingApp, "existing"),
				).
				Value(&mode),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form1.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return nil, output.ErrBare(1)
		}
		return nil, err
	}

	if mode == "existing" {
		return runExistingAppForm(f, msg)
	}

	return runCreateAppFlow(ctx, f, "", msg)
}

// runExistingAppForm shows a huh form for manually entering App ID / App Secret / Brand.
func runExistingAppForm(f *cmdutil.Factory, msg *initMsg) (*configInitResult, error) {
	// Load existing config for defaults
	existing, _ := core.LoadMultiAppConfig()
	var firstApp *core.AppConfig
	if existing != nil {
		firstApp = existing.CurrentAppConfig("")
	}

	var appID, appSecret, brand string

	appIDInput := huh.NewInput().
		Title("App ID").
		Value(&appID)
	if firstApp != nil && firstApp.AppId != "" {
		appIDInput = appIDInput.Placeholder(firstApp.AppId)
	} else {
		appIDInput = appIDInput.Placeholder("cli_xxxx")
	}

	appSecretInput := huh.NewInput().
		Title("App Secret").
		EchoMode(huh.EchoModePassword).
		Value(&appSecret)
	if firstApp != nil && !firstApp.AppSecret.IsZero() {
		appSecretInput = appSecretInput.Placeholder("****")
	} else {
		appSecretInput = appSecretInput.Placeholder("xxxx")
	}

	brand = "feishu"
	if firstApp != nil && firstApp.Brand != "" {
		brand = string(firstApp.Brand)
	}

	form := huh.NewForm(
		huh.NewGroup(
			appIDInput,
			appSecretInput,
			huh.NewSelect[string]().
				Title(msg.Platform).
				Options(
					huh.NewOption(msg.Feishu, "feishu"),
					huh.NewOption("Lark", "lark"),
				).
				Value(&brand),
		),
	).WithTheme(cmdutil.ThemeFeishu())

	if err := form.Run(); err != nil {
		if err == huh.ErrUserAborted {
			return nil, output.ErrBare(1)
		}
		return nil, err
	}

	// Resolve defaults
	if appID == "" && firstApp != nil {
		appID = firstApp.AppId
	}
	if appSecret == "" && firstApp != nil && !firstApp.AppSecret.IsZero() {
		// Keep existing secret - caller will handle
		return &configInitResult{
			Mode:  "existing",
			Brand: parseBrand(brand),
			AppID: appID,
		}, nil
	}

	switch {
	case appID == "" && appSecret == "":
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID and App Secret cannot be empty").
			WithParam("--app-id")
	case appID == "":
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID cannot be empty").
			WithParam("--app-id")
	case appSecret == "":
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "App Secret cannot be empty").
			WithParam("--app-secret")
	}

	return &configInitResult{
		Mode:      "existing",
		Brand:     parseBrand(brand),
		AppID:     appID,
		AppSecret: appSecret,
	}, nil
}

// runCreateAppFlow runs the "create new app" flow via OpenClaw device flow.
// If brandOverride is non-empty, skip the interactive brand selection.
func runCreateAppFlow(ctx context.Context, f *cmdutil.Factory, brandOverride core.LarkBrand, msg *initMsg) (*configInitResult, error) {
	var larkBrand core.LarkBrand
	if brandOverride != "" {
		larkBrand = brandOverride
	} else {
		// Phase 2: Brand selection
		var brand string
		form2 := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title(msg.SelectPlatform).
					Options(
						huh.NewOption(msg.Feishu, "feishu"),
						huh.NewOption("Lark", "lark"),
					).
					Value(&brand),
			),
		).WithTheme(cmdutil.ThemeFeishu())

		if err := form2.Run(); err != nil {
			if err == huh.ErrUserAborted {
				return nil, output.ErrBare(1)
			}
			return nil, err
		}
		larkBrand = parseBrand(brand)
	}

	// Step 1: Request app registration (begin)
	// Use the shared proxy-plugin-aware transport so registration traffic is not
	// a bypass of proxy plugin mode.
	httpClient := transport.NewHTTPClient(0)
	authResp, err := larkauth.RequestAppRegistration(ctx, httpClient, larkBrand, f.IOStreams.ErrOut)
	if err != nil {
		return nil, classifyRegistrationBeginError(err)
	}

	// Step 2: Build and display verification URL + QR code
	verificationURL := larkauth.BuildVerificationURL(authResp.VerificationUriComplete, build.Version)

	// Branch on TTY: human-friendly copy in interactive terminals,
	// preserve original copy for AI / non-interactive callers.
	if f.IOStreams.IsTerminal {
		fmt.Fprintf(f.IOStreams.ErrOut, "%s", msg.ScanQRCode)
		qr, qrErr := qrcode.New(verificationURL, qrcode.Medium)
		if qrErr == nil {
			fmt.Fprint(f.IOStreams.ErrOut, qr.ToSmallString(false))
		}
		fmt.Fprintf(f.IOStreams.ErrOut, "%s", msg.ScanOrOpenLink)
		fmt.Fprintf(f.IOStreams.ErrOut, "  %s\n\n", verificationURL)
		fmt.Fprintf(f.IOStreams.ErrOut, "%s\n", msg.WaitingForScan)
	} else {
		qr, qrErr := qrcode.New(verificationURL, qrcode.Medium)
		if qrErr == nil {
			fmt.Fprint(f.IOStreams.ErrOut, qr.ToSmallString(false))
		}
		fmt.Fprintf(f.IOStreams.ErrOut, "%s", msg.OpenLinkNonTTY)
		fmt.Fprintf(f.IOStreams.ErrOut, "  %s\n\n", verificationURL)
		fmt.Fprintf(f.IOStreams.ErrOut, "%s\n", msg.WaitingForScanNonTTY)
	}
	// Step 4: Poll for credentials (brand discovery lives in internal/auth);
	// this layer only classifies the terminal error and saves the result.
	result, finalBrand, err := larkauth.RegisterAppWithDiscovery(ctx, httpClient, authResp, f.IOStreams.ErrOut)
	if err != nil {
		return nil, classifyRegistrationError(err)
	}

	if result.ClientID == "" || result.ClientSecret == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_id or client_secret")
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.AppCreated, result.ClientID))

	return &configInitResult{
		Mode:      "create",
		Brand:     finalBrand,
		AppID:     result.ClientID,
		AppSecret: result.ClientSecret,
	}, nil
}

// classifyRegistrationBeginError keeps transport/cancellation failures out of
// the invalid-client category: the begin request sends no app credentials.
func classifyRegistrationBeginError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "app registration cancelled").WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return errs.NewNetworkError(errs.SubtypeNetworkTimeout, "app registration begin timed out: %v", err).WithCause(err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		subtype := errs.SubtypeNetworkTransport
		if netErr.Timeout() {
			subtype = errs.SubtypeNetworkTimeout
		}
		return errs.NewNetworkError(subtype, "app registration begin failed: %v", err).WithCause(err)
	}
	return errs.NewAPIError(errs.SubtypeUnknown, "app registration begin failed: %v", err).WithCause(err)
}

// classifyRegistrationError maps registration terminal outcomes to typed
// errors, preserving causes.
func classifyRegistrationError(err error) error {
	switch {
	case errors.Is(err, larkauth.ErrRegistrationDenied):
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "%v", err).
			WithHint("re-run `lark-cli config init --new` and approve the authorization request").
			WithCause(err)
	case errors.Is(err, larkauth.ErrRegistrationExpired), errors.Is(err, larkauth.ErrRegistrationTimedOut):
		return errs.NewAuthenticationError(errs.SubtypeTokenExpired, "%v", err).
			WithHint("re-run `lark-cli config init --new` and complete the scan before the code expires").
			WithCause(err)
	default:
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "app registration failed: %v", err).WithCause(err)
	}
}
