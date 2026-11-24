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
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/keylesshelper"
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
	AuthMethod string // "" == client_secret; private_key_jwt_local_keypair
	KeyRef     *core.KeyRef
	KeyID      string // RFC 7638 thumbprint returned for the registered public key
	Signer     keysigner.Signer
}

// runInteractiveConfigInit shows an interactive TUI for config init.
func runInteractiveConfigInit(ctx context.Context, f *cmdutil.Factory, authMethod string, msg *initMsg, store *keylesshelper.KeyStore, defaults *core.AppConfig) (*configInitResult, error) {
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
		return runExistingAppForm(ctx, f, authMethod, msg, store, defaults)
	}

	return runCreateAppFlow(ctx, f, "", authMethod, msg, "", store)
}

// runExistingAppForm shows a huh form for manually entering App ID / App Secret / Brand.
func runExistingAppForm(ctx context.Context, f *cmdutil.Factory, requestedAuthMethod string, msg *initMsg, store *keylesshelper.KeyStore, defaults *core.AppConfig) (*configInitResult, error) {
	var appID, appSecret, brand string
	requiresSecret := requestedAuthMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair

	appIDInput := huh.NewInput().
		Title("App ID").
		Value(&appID)
	if defaults != nil && defaults.AppId != "" {
		appIDInput = appIDInput.Placeholder(defaults.AppId)
	} else {
		appIDInput = appIDInput.Placeholder("cli_xxxx")
	}

	appSecretInput := huh.NewInput().
		Title("App Secret").
		EchoMode(huh.EchoModePassword).
		Value(&appSecret)
	if defaults != nil && !defaults.AppSecret.IsZero() {
		appSecretInput = appSecretInput.Placeholder("****")
	} else {
		appSecretInput = appSecretInput.Placeholder("xxxx")
	}

	brand = "feishu"
	if defaults != nil && defaults.Brand != "" {
		brand = string(defaults.Brand)
	}

	brandSelect := huh.NewSelect[string]().
		Title(msg.Platform).
		Options(
			huh.NewOption(msg.Feishu, "feishu"),
			huh.NewOption("Lark", "lark"),
		).
		Value(&brand)

	var form *huh.Form
	if requiresSecret {
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
	appID = strings.TrimSpace(appID)
	if appID == "" && defaults != nil {
		appID = defaults.AppId
	}
	if !requiresSecret {
		if appID == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "App ID cannot be empty").
				WithParam("--app-id")
		}
		return runCreateAppFlow(ctx, f, parseBrand(brand), requestedAuthMethod, msg, appID, store)
	}
	if appSecret == "" && defaults != nil && !defaults.AppSecret.IsZero() {
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

func probeRegistrationSigner(ctx context.Context, store *keylesshelper.KeyStore) error {
	err := store.ProbeWritableContext(ctx)
	if err == nil || errs.IsTyped(err) {
		return err
	}
	return errs.NewConfigError(errs.SubtypeInvalidClient, "this machine does not support --private-key-jwt").
		WithCause(err).
		WithHint("omit --private-key-jwt to register with an app secret")
}

// runCreateAppFlow runs the "create new app" flow via OpenClaw device flow.
// If brandOverride is non-empty, skip the interactive brand selection.
// targetAppID, when non-empty, identifies an existing app being migrated to
// the requested authentication method. Empty preserves the normal new-app flow.
func runCreateAppFlow(ctx context.Context, f *cmdutil.Factory, brandOverride core.LarkBrand, authMethod string, msg *initMsg, targetAppID string, store *keylesshelper.KeyStore) (_ *configInitResult, retErr error) {
	var registrationKey *keylesshelper.Key
	retainRegistrationKey := false
	defer func() {
		if registrationKey == nil || retainRegistrationKey {
			return
		}
		cleanupCtx := ctx
		if cleanupCtx == nil {
			cleanupCtx = context.Background()
		} else {
			cleanupCtx = context.WithoutCancel(cleanupCtx)
		}
		if err := registrationKey.DeleteContext(cleanupCtx); err != nil {
			cleanupErr := errs.NewInternalError(
				errs.SubtypeStorage,
				"failed to clean up uncommitted registration key: %v",
				err,
			).WithCause(err)
			retErr = errors.Join(retErr, cleanupErr)
		}
	}()

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

	// Step 1: Request app registration (begin).
	// Use the shared proxy-plugin-aware transport so registration traffic is not
	// a bypass of proxy plugin mode.
	httpClient := transport.NewHTTPClient(0)

	// For private_key_jwt_local_keypair, obtain a nonce and sign an attestation
	// carrying the public key in its jwk header to send with begin.
	beginOpts := larkauth.AppRegistrationBeginOptions{AuthMethod: authMethod, TargetAppID: targetAppID}
	initResult := &configInitResult{Mode: "create", AuthMethod: authMethod}
	if authMethod == core.AuthMethodPrivateKeyJWTLocalKeyPair {
		initResp, initErr := larkauth.RequestAppRegistrationInit(ctx, httpClient)
		if initErr != nil {
			return nil, classifyRegistrationRequestError("init", initErr)
		}
		// An empty SupportedAuthMethods is treated as unknown for compatibility.
		// An explicit capability list must contain the requested auth method.
		if len(initResp.SupportedAuthMethods) > 0 &&
			!slices.Contains(initResp.SupportedAuthMethods, authMethod) {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient,
				"server does not support %s for this app type (supported: %s)", authMethod, strings.Join(initResp.SupportedAuthMethods, ", ")).
				WithHint("omit --private-key-jwt to register with an app secret instead")
		}
		keyLabel, initErr := keysigner.NewKeyLabel("larksuite-cli-")
		if initErr != nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to allocate registration key: %v", initErr).WithCause(initErr)
		}
		var signErr error
		registrationKey, beginOpts.AuthAttestation, signErr = store.CreateAttestationContext(ctx, keyLabel, initResp.Nonce, time.Now())
		if signErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "failed to prepare registration key attestation: %v", signErr).WithCause(signErr)
		}
		initResult.KeyID, signErr = registrationKey.Thumbprint()
		if signErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "failed to identify registration public key: %v", signErr).WithCause(signErr)
		}
		initResult.KeyRef = &core.KeyRef{Source: core.KeySourceTEE, Provider: registrationKey.Provider(), ID: registrationKey.ID()}
		initResult.Signer = registrationKey.Signer()
	}

	authResp, err := larkauth.RequestAppRegistration(ctx, httpClient, larkBrand, beginOpts, f.IOStreams.ErrOut)
	if err != nil {
		return nil, classifyRegistrationRequestError("begin", err)
	}

	// Step 2: Build and display verification URL + QR code
	verificationURL := larkauth.BuildVerificationURL(authResp.VerificationUriComplete, build.Version, targetAppID)

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
		return nil, classifyRegistrationError(err, authMethod, targetAppID)
	}

	if result.ClientID == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_id")
	}
	if targetAppID != "" && result.ClientID != targetAppID {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient,
			"app registration returned client_id %q for requested app %q", result.ClientID, targetAppID)
	}
	if authMethod != core.AuthMethodPrivateKeyJWTLocalKeyPair && result.ClientSecret == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_secret")
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.AppCreated, result.ClientID))

	retainRegistrationKey = true
	initResult.Brand = finalBrand
	initResult.AppID = result.ClientID
	initResult.AppSecret = result.ClientSecret
	return initResult, nil
}

// classifyRegistrationRequestError classifies the init and begin exchanges.
func classifyRegistrationRequestError(action string, err error) error {
	if errs.IsTyped(err) {
		return err
	}
	var remoteErr *larkauth.AppRegistrationRemoteError
	if errors.As(err, &remoteErr) {
		return classifyRegistrationError(err, "", "")
	}
	switch {
	case errors.Is(err, context.Canceled):
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "app registration cancelled").WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return errs.NewNetworkError(errs.SubtypeNetworkTimeout, "app registration %s timed out: %v", action, err).WithCause(err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		subtype := errs.SubtypeNetworkTransport
		if netErr.Timeout() {
			subtype = errs.SubtypeNetworkTimeout
		}
		return errs.NewNetworkError(subtype, "app registration %s failed: %v", action, err).WithCause(err)
	}
	return errs.NewAPIError(errs.SubtypeUnknown, "app registration %s failed: %v", action, err).WithCause(err)
}

// classifyRegistrationError preserves the cause and the registration context
// needed to retry the same authentication method and target app.
func classifyRegistrationError(err error, authMethod, targetAppID string) error {
	var remoteErr *larkauth.AppRegistrationRemoteError
	if errors.As(err, &remoteErr) {
		if errclass.IsClientAssertionPublicKeyError(remoteErr.Code) {
			classified := errclass.BuildAPIError(map[string]any{
				"code": remoteErr.Code,
				"msg":  remoteErr.Error(),
			}, errclass.ClassifyContext{})
			var apiErr *errs.APIError
			if errors.As(classified, &apiErr) {
				return apiErr.WithCause(err)
			}
			var permissionErr *errs.PermissionError
			if errors.As(classified, &permissionErr) {
				return permissionErr.WithCause(err)
			}
		}
		return errs.NewAPIError(errs.SubtypeUnknown, "app registration failed: %s", remoteErr.Error()).
			WithCode(remoteErr.Code).
			WithCause(err)
	}
	retryCommand := "lark-cli config init"
	if targetAppID == "" {
		retryCommand += " --new"
	}
	if authMethod == core.AuthMethodPrivateKeyJWTLocalKeyPair {
		retryCommand += " --private-key-jwt"
	}
	retryHint := fmt.Sprintf("re-run `%s`", retryCommand)
	if targetAppID != "" {
		retryHint += fmt.Sprintf(" in a terminal, choose the existing-app option, and enter app ID %s", targetAppID)
	}
	switch {
	case errors.Is(err, larkauth.ErrRegistrationDeniedByUser):
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "%v", err).
			WithHint("%s and approve the authorization request", retryHint).
			WithCause(err)
	case errors.Is(err, larkauth.ErrRegistrationExpired), errors.Is(err, larkauth.ErrRegistrationTimedOut):
		return errs.NewAuthenticationError(errs.SubtypeTokenExpired, "%v", err).
			WithHint("%s and complete the scan before the code expires", retryHint).
			WithCause(err)
	default:
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "app registration failed: %v", err).WithCause(err)
	}
}
