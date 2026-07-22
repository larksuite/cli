// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/larksuite/cli/internal/build"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/transport"
)

// configInitResult holds the result of the interactive config init flow.
type configInitResult struct {
	Mode       string // "create" or "existing"
	Brand      core.LarkBrand
	AppID      string
	AppSecret  string
	AuthMethod string // "" == client_secret; core.AuthMethodPrivateKeyJWT
	KeyLabel   string // TEE key handle when AuthMethod == private_key_jwt
}

// runInteractiveConfigInit shows an interactive TUI for config init.
func runInteractiveConfigInit(ctx context.Context, f *cmdutil.Factory, authMethodFlag string, msg *initMsg) (*configInitResult, error) {
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
		return runExistingAppForm(ctx, f, authMethodFlag, msg)
	}

	return runCreateAppFlow(ctx, f, "", authMethodFlag, msg, "")
}

func existingAppRequiresSecret(requestedAuthMethod string) bool {
	return requestedAuthMethod != core.AuthMethodPrivateKeyJWT
}

// runExistingAppForm shows a huh form for manually entering App ID / App Secret / Brand.
func runExistingAppForm(ctx context.Context, f *cmdutil.Factory, requestedAuthMethod string, msg *initMsg) (*configInitResult, error) {
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

	brandSelect := huh.NewSelect[string]().
		Title(msg.Platform).
		Options(
			huh.NewOption(msg.Feishu, "feishu"),
			huh.NewOption("Lark", "lark"),
		).
		Value(&brand)

	var form *huh.Form
	if existingAppRequiresSecret(requestedAuthMethod) {
		form = huh.NewForm(
			huh.NewGroup(
				appIDInput,
				appSecretInput,
				brandSelect,
			),
		).WithTheme(cmdutil.ThemeFeishu())
	} else {
		form = huh.NewForm(
			huh.NewGroup(
				appIDInput,
				brandSelect,
			),
		).WithTheme(cmdutil.ThemeFeishu())
	}

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
	if !existingAppRequiresSecret(requestedAuthMethod) {
		if appID == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID cannot be empty").
				WithParam("--app-id")
		}
		return runCreateAppFlow(ctx, f, parseBrand(brand), core.AuthMethodPrivateKeyJWT, msg, appID)
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

// resolveRegisterAuthMethod decides the auth method for a new-app registration.
// An explicit private_key_jwt request wins; otherwise the default is
// client_secret with no extra prompt.
func resolveRegisterAuthMethod(ctx context.Context, _ *cmdutil.Factory, requested string) (string, error) {
	const pkjwtUnsupportedMessage = "this machine does not support --private-key-jwt"

	switch requested {
	case core.AuthMethodPrivateKeyJWT:
		info, ok, err := keysigner.ProbeActiveHardware(ctx)
		if !ok {
			return "", errs.NewConfigError(errs.SubtypeInvalidClient,
				pkjwtUnsupportedMessage).
				WithHint("omit --private-key-jwt to register with an app secret")
		}
		if err != nil {
			return "", errs.NewConfigError(errs.SubtypeInvalidClient,
				pkjwtUnsupportedMessage).
				WithCause(err).
				WithHint("omit --private-key-jwt to register with an app secret")
		}
		if !info.Available {
			return "", errs.NewConfigError(errs.SubtypeInvalidClient,
				pkjwtUnsupportedMessage).
				WithHint("omit --private-key-jwt to register with an app secret")
		}
		return core.AuthMethodPrivateKeyJWT, nil
	case core.AuthMethodClientSecret:
		return core.AuthMethodClientSecret, nil
	case "":
		return core.AuthMethodClientSecret, nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown auth method %q (use client_secret or private_key_jwt)", requested)
	}
}

// runCreateAppFlow runs the "create new app" flow via OpenClaw device flow.
// If brandOverride is non-empty, skip the interactive brand selection.
// requestedAuthMethod is the requested auth method; empty means client_secret.
// restoreAppID, when non-empty, is sent on the registration begin request so the
// server re-registers that existing app (credential recovery) instead of creating
// a new one. Empty preserves the normal new-app flow.
func runCreateAppFlow(ctx context.Context, f *cmdutil.Factory, brandOverride core.LarkBrand, requestedAuthMethod string, msg *initMsg, restoreAppID string) (*configInitResult, error) {
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

	authMethod, err := resolveRegisterAuthMethod(ctx, f, requestedAuthMethod)
	if err != nil {
		return nil, err
	}

	// Step 1: Request app registration (begin).
	// Use the shared proxy-plugin-aware transport so registration traffic is not
	// a bypass of proxy plugin mode.
	httpClient := transport.NewHTTPClient(0)

	// For private_key_jwt: init to obtain a nonce, then sign a TEE attestation
	// (carrying the public key in its jwk header) to send with begin.
	beginOpts := larkauth.AppRegistrationBeginOptions{}
	keyLabel := ""
	if authMethod == core.AuthMethodPrivateKeyJWT {
		initResp, initErr := larkauth.RequestAppRegistrationInit(ctx, httpClient)
		if initErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration init failed: %v", initErr).WithCause(initErr)
		}
		// An empty SupportedAuthMethods is intentionally treated as "older server /
		// unknown": len()==0 makes this guard false, so the requested
		// private_key_jwt proceeds. This mirrors resolveFinalAuthMethod's
		// back-compat fallback to the requested method. Only an explicit list that
		// omits private_key_jwt rejects here.
		if len(initResp.SupportedAuthMethods) > 0 && !slices.Contains(initResp.SupportedAuthMethods, core.AuthMethodPrivateKeyJWT) {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient,
				"server does not support private_key_jwt for this app type (supported: %s)", strings.Join(initResp.SupportedAuthMethods, ", ")).
				WithHint("omit --private-key-jwt to register with an app secret instead")
		}
		keyLabel = keysigner.DefaultKeyLabel
		signer := keysigner.Active() // non-nil, guaranteed by resolveRegisterAuthMethod
		attestation, signErr := jwt.SignAttestation(ctx, signer, keysigner.KeyRef{Label: keyLabel}, initResp.Nonce, time.Now())
		if signErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "failed to sign registration attestation: %v", signErr).WithCause(signErr)
		}
		beginOpts = larkauth.AppRegistrationBeginOptions{
			AuthMethod:      core.AuthMethodPrivateKeyJWT,
			AuthAttestation: attestation,
		}
	}

	// Restore flow: re-register the existing app instead of creating a new one.
	beginOpts.RestoreAppID = restoreAppID

	authResp, err := larkauth.RequestAppRegistration(ctx, httpClient, larkBrand, beginOpts, f.IOStreams.ErrOut)
	if err != nil {
		return nil, classifyRegistrationBeginError(err)
	}

	// Step 2: Build and display verification URL + QR code
	verificationURL := larkauth.BuildVerificationURL(authResp.VerificationUriComplete, build.Version, restoreAppID)

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

	// The final auth method is decided by the user/admin at confirmation and
	// returned by poll — NOT necessarily what we requested. Selecting an existing
	// client_secret app, for example, yields client_secret even though we sent
	// private_key_jwt. Trust the result so we persist the truth.
	finalMethod := resolveFinalAuthMethod(result.AuthMethods, authMethod)

	if result.ClientID == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing app_id")
	}
	if finalMethod != core.AuthMethodPrivateKeyJWT && result.ClientSecret == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_secret")
	}

	// Surface a downgrade: requested private_key_jwt but the app resolved to a
	// secret-based method (e.g. an existing app was selected). The key was NOT
	// bound, so we must store the secret method, not private_key_jwt.
	if authMethod == core.AuthMethodPrivateKeyJWT && finalMethod != core.AuthMethodPrivateKeyJWT {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] note: requested private_key_jwt, but the app uses %q (e.g. an existing app was selected); storing %q.\n", finalMethod, finalMethod)
	}
	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.AppCreated, result.ClientID))

	keyToStore := ""
	if finalMethod == core.AuthMethodPrivateKeyJWT {
		keyToStore = keyLabel
	}
	if err := validatePKJWTKeyBinding(finalMethod, keyToStore); err != nil {
		return nil, err
	}
	return &configInitResult{
		Mode:       "create",
		Brand:      finalBrand,
		AppID:      result.ClientID,
		AppSecret:  result.ClientSecret, // empty for private_key_jwt; real secret otherwise
		AuthMethod: finalMethod,
		KeyLabel:   keyToStore,
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

// validatePKJWTKeyBinding rejects a registration that resolved to
// private_key_jwt without a signing key bound to it. keyLabel is non-empty only
// when the local flow chose private_key_jwt and signed a TEE attestation; a
// resolved method of private_key_jwt with no key handle would save an unusable
// config (rejected later at config load, surfacing as "saved OK, fails on first
// use"), so it is caught here at registration time instead.
func validatePKJWTKeyBinding(finalMethod, keyLabel string) error {
	if finalMethod == core.AuthMethodPrivateKeyJWT && keyLabel == "" {
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"registration resolved to private_key_jwt but no signing key was bound to this app (an existing secret-based app may have been selected)").
			WithHint("re-register with: lark-cli config init --new --private-key-jwt")
	}
	return nil
}

// resolveFinalAuthMethod picks the authoritative method from the poll result,
// preferring private_key_jwt, then client_secret. It falls back to the requested
// method when the server returns nothing (older servers).
func resolveFinalAuthMethod(serverMethods []string, requested string) string {
	if len(serverMethods) == 0 {
		if requested == "" {
			return core.AuthMethodClientSecret
		}
		return requested
	}
	for _, m := range serverMethods {
		if m == core.AuthMethodPrivateKeyJWT {
			return core.AuthMethodPrivateKeyJWT
		}
	}
	for _, m := range serverMethods {
		if m == core.AuthMethodClientSecret {
			return core.AuthMethodClientSecret
		}
	}
	return serverMethods[0]
}
