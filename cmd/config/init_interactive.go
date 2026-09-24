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
	"github.com/larksuite/cli/internal/keylesshelper"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/transport"
)

// configInitResult holds the result of the interactive config init flow.
type configInitResult struct {
	Mode        string // "create" or "existing"
	Brand       core.LarkBrand
	AppID       string
	AppSecret   string
	AuthMethod  string // "" == client_secret; either private-key JWT method
	KeySource   string // tee or file
	KeyLabel    string // signer handle when AuthMethod uses private-key JWT
	KeyProvider string // exact built-in backend; empty for imported/external keys
	KeyID       string // RFC 7638 thumbprint returned for the registered public key
	Signer      keysigner.Signer
}

// runInteractiveConfigInit shows an interactive TUI for config init.
func runInteractiveConfigInit(ctx context.Context, f *cmdutil.Factory, authMethodFlag string, msg *initMsg, registrationSigners []keysigner.Signer) (*configInitResult, error) {
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
		return runExistingAppForm(ctx, f, authMethodFlag, msg, registrationSigners)
	}

	return runCreateAppFlow(ctx, f, "", authMethodFlag, msg, "", registrationSigners)
}

func existingAppRequiresSecret(requestedAuthMethod string) bool {
	return !core.IsPrivateKeyJWTAuthMethod(requestedAuthMethod)
}

// runExistingAppForm shows a huh form for manually entering App ID / App Secret / Brand.
func runExistingAppForm(ctx context.Context, f *cmdutil.Factory, requestedAuthMethod string, msg *initMsg, registrationSigners []keysigner.Signer) (*configInitResult, error) {
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
		return runCreateAppFlow(ctx, f, parseBrand(brand), requestedAuthMethod, msg, appID, registrationSigners)
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
// An explicit private-key JWT request wins; otherwise the default is
// client_secret with no extra prompt.
func resolveRegisterAuthMethod(ctx context.Context, requested string, registrationSigners []keysigner.Signer) (string, error) {
	const pkjwtUnsupportedMessage = "this machine does not support --private-key-jwt"

	switch requested {
	case core.AuthMethodPrivateKeyJWT:
		ok, err := probeRegistrationSigner(ctx, registrationSigners)
		if !ok {
			return "", errs.NewConfigError(errs.SubtypeInvalidClient,
				pkjwtUnsupportedMessage).
				WithHint("omit --private-key-jwt to register with an app secret")
		}
		if err != nil {
			if errs.IsTyped(err) {
				return "", err
			}
			return "", errs.NewConfigError(errs.SubtypeInvalidClient,
				pkjwtUnsupportedMessage).
				WithCause(err).
				WithHint("omit --private-key-jwt to register with an app secret")
		}
		return requested, nil
	case core.AuthMethodPrivateKeyJWTLocalKeyPair:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s uses an already-registered private key file", requested).
			WithHint("use config init --app-id <app-id> --private-key-file <path>")
	case core.AuthMethodClientSecret:
		return core.AuthMethodClientSecret, nil
	case "":
		return core.AuthMethodClientSecret, nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown registration auth method %q (use client_secret or private_key_jwt)", requested)
	}
}

func probeRegistrationSigner(ctx context.Context, signers []keysigner.Signer) (bool, error) {
	if len(signers) == 0 {
		return false, nil
	}
	return true, keylesshelper.NewKeyStoreWithSigners(nil, signers...).ProbeWritableContext(ctx)
}

// runCreateAppFlow runs the "create new app" flow via OpenClaw device flow.
// If brandOverride is non-empty, skip the interactive brand selection.
// requestedAuthMethod is the requested auth method; empty means client_secret.
// targetAppID, when non-empty, identifies an existing app being migrated to
// the requested authentication method. Empty preserves the normal new-app flow.
func runCreateAppFlow(ctx context.Context, f *cmdutil.Factory, brandOverride core.LarkBrand, requestedAuthMethod string, msg *initMsg, targetAppID string, registrationSigners []keysigner.Signer) (_ *configInitResult, retErr error) {
	var registrationKey *keylesshelper.Key
	var registrationSigner keysigner.Signer
	store := keylesshelper.NewKeyStoreWithSigners(f.Keychain, registrationSigners...)
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
		if err := store.DeleteKeyContext(cleanupCtx, registrationKey); err != nil &&
			!errors.Is(err, keysigner.ErrKeyNotFound) {
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

	authMethod, err := resolveRegisterAuthMethod(ctx, requestedAuthMethod, registrationSigners)
	if err != nil {
		return nil, err
	}

	// Step 1: Request app registration (begin).
	// Use the shared proxy-plugin-aware transport so registration traffic is not
	// a bypass of proxy plugin mode.
	httpClient := transport.NewHTTPClient(0)

	// For private_key_jwt: init to obtain a nonce, then sign a managed-key attestation
	// (carrying the public key in its jwk header) to send with begin.
	beginOpts := larkauth.AppRegistrationBeginOptions{}
	keyLabel := ""
	keyProvider := ""
	keyID := ""
	if authMethod == core.AuthMethodPrivateKeyJWT {
		initResp, initErr := larkauth.RequestAppRegistrationInit(ctx, httpClient)
		if initErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration init failed: %v", initErr).WithCause(initErr)
		}
		// An empty SupportedAuthMethods is treated as unknown for compatibility.
		// An explicit capability list must contain the requested method. The
		// two private-key methods are distinct and must never replace each other.
		if len(initResp.SupportedAuthMethods) > 0 &&
			!slices.Contains(initResp.SupportedAuthMethods, authMethod) {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient,
				"server does not support %s for this app type (supported: %s)", authMethod, strings.Join(initResp.SupportedAuthMethods, ", ")).
				WithHint("omit --private-key-jwt to register with an app secret instead")
		}
		keyLabel, initErr = keysigner.NewKeyLabel("larksuite-cli-")
		if initErr != nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to allocate registration key: %v", initErr).WithCause(initErr)
		}
		var attestation string
		var signErr error
		registrationKey, attestation, signErr = store.CreateAttestationContext(ctx, keyLabel, initResp.Nonce, time.Now())
		if signErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "failed to prepare registration key attestation: %v", signErr).WithCause(signErr)
		}
		for _, signer := range registrationSigners {
			if signer.Name() == registrationKey.Provider() {
				registrationSigner = signer
				break
			}
		}
		keyProvider = registrationKey.Provider()
		keyID, signErr = registrationKey.Thumbprint()
		if signErr != nil {
			return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "failed to identify registration public key: %v", signErr).WithCause(signErr)
		}
		beginOpts = larkauth.AppRegistrationBeginOptions{
			AuthMethod:      authMethod,
			AuthAttestation: attestation,
		}
	}

	beginOpts.TargetAppID = targetAppID

	authResp, err := larkauth.RequestAppRegistration(ctx, httpClient, larkBrand, beginOpts, f.IOStreams.ErrOut)
	if err != nil {
		return nil, classifyRegistrationBeginError(err)
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
		return nil, classifyRegistrationError(err)
	}

	if authMethod == "client_secret_basic" || authMethod == "client_secret_post" {
		authMethod = core.AuthMethodClientSecret
	}
	switch authMethod {
	case core.AuthMethodClientSecret, core.AuthMethodPrivateKeyJWT:
	default:
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient,
			"app registration resolved unsupported auth method %q", authMethod)
	}

	if result.ClientID == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_id")
	}
	if !core.IsPrivateKeyJWTAuthMethod(authMethod) && result.ClientSecret == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_secret")
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.AppCreated, result.ClientID))

	keyToStore := ""
	if core.IsPrivateKeyJWTAuthMethod(authMethod) {
		keyToStore = keyLabel
	}
	if err := validatePKJWTKeyBinding(authMethod, keyToStore); err != nil {
		return nil, err
	}
	retainRegistrationKey = core.IsPrivateKeyJWTAuthMethod(authMethod)
	return &configInitResult{
		Mode:        "create",
		Brand:       finalBrand,
		AppID:       result.ClientID,
		AppSecret:   result.ClientSecret, // empty for private-key JWT; real secret otherwise
		AuthMethod:  authMethod,
		KeySource:   core.SecretSourceTEE,
		KeyLabel:    keyToStore,
		KeyProvider: keyProvider,
		KeyID:       keyID,
		Signer:      registrationSigner,
	}, nil
}

// classifyRegistrationBeginError keeps transport/cancellation failures out of
// the invalid-client category: the begin request sends no app credentials.
func classifyRegistrationBeginError(err error) error {
	var remoteErr *larkauth.AppRegistrationRemoteError
	if errors.As(err, &remoteErr) {
		return classifyRegistrationError(err)
	}
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
	var remoteErr *larkauth.AppRegistrationRemoteError
	if errors.As(err, &remoteErr) {
		switch remoteErr.Code {
		case larkauth.AppRegistrationCodeInvalidPublicKey:
			detail := remoteErr.Description
			if detail == "" {
				detail = "invalid public key or incompatible enterprise keyless policy"
			}
			return errs.NewAPIError(errs.SubtypeInvalidParameters,
				"the platform rejected the registration public key: %s", detail).
				WithCode(remoteErr.Code).
				WithHint("review the app's keyless authentication policy and registered public keys in Developer Console, then retry").
				WithCause(err)
		case larkauth.AppRegistrationCodePublicKeyLimit:
			return errs.NewAPIError(errs.SubtypeQuotaExceeded,
				"the app has reached its public-key limit").
				WithCode(remoteErr.Code).
				WithHint("delete an unused public key in Developer Console, then retry").
				WithCause(err)
		default:
			return errs.NewAPIError(errs.SubtypeUnknown, "app registration failed: %s", remoteErr.Error()).
				WithCode(remoteErr.Code).
				WithCause(err)
		}
	}
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

// validatePKJWTKeyBinding rejects a private-key JWT registration without the
// key that signed its attestation. Persisting such a config would defer the
// failure until the first token request.
func validatePKJWTKeyBinding(authMethod, keyLabel string) error {
	if core.IsPrivateKeyJWTAuthMethod(authMethod) && keyLabel == "" {
		return errs.NewConfigError(errs.SubtypeInvalidClient,
			"registration resolved to %s but no signing key was bound to this app (an existing secret-based app may have been selected)", authMethod).
			WithHint("re-register with: lark-cli config init --new --private-key-jwt")
	}
	return nil
}
