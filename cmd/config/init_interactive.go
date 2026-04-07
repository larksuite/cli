// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"
	"net/http"

	"github.com/charmbracelet/huh"
	"github.com/larksuite/cli/internal/build"
	qrcode "github.com/skip2/go-qrcode"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
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

	if appID == "" || appSecret == "" {
		return nil, output.ErrValidation("App ID and App Secret cannot be empty")
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
	httpClient := &http.Client{}
	authResp, err := larkauth.RequestAppRegistration(httpClient, larkBrand, f.IOStreams.ErrOut)
	if err != nil {
		return nil, output.ErrAuth("app registration failed: %v", err)
	}

	// Step 2: Build and display verification URL + QR code
	verificationURL := larkauth.BuildVerificationURL(authResp.VerificationUriComplete, build.Version)

	// Show QR code in terminal
	qr, qrErr := qrcode.New(verificationURL, qrcode.Medium)
	if qrErr == nil {
		fmt.Fprint(f.IOStreams.ErrOut, qr.ToSmallString(false))
	}

	fmt.Fprintf(f.IOStreams.ErrOut, "%s", msg.ScanOrOpenLink)
	fmt.Fprintf(f.IOStreams.ErrOut, "  %s\n\n", verificationURL)

	// Step 3: Poll for result
	fmt.Fprintf(f.IOStreams.ErrOut, "%s\n", msg.WaitingForScan)
	result, err := larkauth.PollAppRegistration(ctx, httpClient, core.BrandFeishu, authResp.DeviceCode, authResp.Interval, authResp.ExpiresIn, f.IOStreams.ErrOut)
	if err != nil {
		return nil, output.ErrAuth("%v", err)
	}

	// Step 4: Handle Lark brand special case
	// If tenant_brand=lark and no client_secret, retry with lark brand endpoint
	if result.ClientSecret == "" && result.UserInfo != nil && result.UserInfo.TenantBrand == "lark" {
		// fmt.Fprintf(f.IOStreams.ErrOut, "%s\n", msg.DetectedLarkTenant)
		result, err = larkauth.PollAppRegistration(ctx, httpClient, core.BrandLark, authResp.DeviceCode, authResp.Interval, authResp.ExpiresIn, f.IOStreams.ErrOut)
		if err != nil {
			return nil, output.ErrAuth("lark endpoint retry failed: %v", err)
		}
	}

	if result.ClientID == "" || result.ClientSecret == "" {
		return nil, output.ErrAuth("app registration succeeded but missing client_id or client_secret")
	}

	// Determine final brand from response
	finalBrand := larkBrand
	if result.UserInfo != nil && result.UserInfo.TenantBrand == "lark" {
		finalBrand = core.BrandLark
	} else if result.UserInfo != nil && result.UserInfo.TenantBrand == "feishu" {
		finalBrand = core.BrandFeishu
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
