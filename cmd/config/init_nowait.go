// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/transport"
)

// newRegistrationHTTPClient builds the HTTP client used for app-registration
// traffic. It is a package var so tests can inject a stub transport.
var newRegistrationHTTPClient = func() *http.Client { return transport.NewHTTPClient(0) }

// initNoWaitHint is the agent-facing guidance embedded in the --no-wait JSON
// output, mirroring the two-step contract of `auth login --no-wait`.
const initNoWaitHint = "**Generate AND display the QR code:** call `lark-cli auth qrcode <verification_url>` and show it (PNG via --output; ASCII via --ascii only if the user asks). " +
	"**You MUST include the QR image in your response** — generating the file alone is not enough. Output the URL first, then the QR image below it. " +
	"**Treat verification_url as an opaque string** — do not URL-encode/decode it or add spaces/punctuation. " +
	"**Hand control back:** make the QR/URL the final message of this turn; do NOT run --device-code in the same turn. Tell the user to come back and notify you after they finish creating the app in the browser. " +
	"**After the user confirms:** YOU must finish by running lark-cli with the exact arguments in `resume_args`, passing each element as a separate literal argument (do not re-quote or shell-interpret them). It already carries the right flags. " +
	"**Do NOT cache verification_url or device_code** — run `lark-cli config init --new --no-wait` fresh whenever a new app is needed."

// initiateNoWaitAppRegistration runs the non-blocking first step: request a
// device code, cache the resume context, print JSON, and return immediately
// without polling.
func initiateNoWaitAppRegistration(opts *ConfigInitOptions, existing *core.MultiAppConfig) error {
	f := opts.Factory
	brand := parseBrand(opts.Brand)

	httpClient := newRegistrationHTTPClient()
	authResp, err := larkauth.RequestAppRegistration(httpClient, brand, f.IOStreams.ErrOut)
	if err != nil {
		// Pass a lower-layer typed error (e.g. a network/transport error) through
		// unchanged; only wrap genuinely-untyped failures as invalid_client.
		if _, ok := errs.ProblemOf(err); ok {
			return err
		}
		return errs.NewConfigError(errs.SubtypeInvalidClient, "app registration failed: %v", err).WithCause(err)
	}

	rec := initNoWaitRecord{
		Version:      initNoWaitCacheVersion,
		Brand:        string(brand),
		ProfileName:  opts.ProfileName,
		Lang:         opts.Lang,
		LangExplicit: opts.langExplicit,
		Interval:     authResp.Interval,
		ExpiresAt:    time.Now().Unix() + int64(authResp.ExpiresIn),
		ConfigDigest: computeConfigDigest(existing),
	}
	// The resume step (--device-code) fully depends on this cache to finish
	// persisting the app — unlike auth login, which can re-derive its scope. So
	// a cache-write failure is fatal: fail now rather than hand back a
	// device_code the user can never complete.
	if err := saveInitNoWaitRecord(authResp.DeviceCode, rec); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to persist the context needed by `config init --device-code`: %v", err).WithCause(err)
	}

	// Emit the resume step as an argv array rather than a shell string: the
	// device_code is opaque and may contain spaces or metacharacters, and a
	// single quoted string can't be both POSIX- and cmd.exe-safe. argv sidesteps
	// quoting entirely — agents pass each element as a literal argument.
	// --force-init must be carried along: guardAgentWorkspace runs in RunE
	// before the cache is read, so resuming without it inside an agent workspace
	// would be rejected. (Profile name is recovered from the cache.)
	resumeArgs := []string{"lark-cli", "config", "init", "--device-code", authResp.DeviceCode}
	if opts.ForceInit {
		resumeArgs = append(resumeArgs, "--force-init")
	}

	verificationURL := larkauth.BuildVerificationURL(authResp.VerificationUriComplete, build.Version)
	data := map[string]interface{}{
		"verification_url": verificationURL,
		"device_code":      authResp.DeviceCode,
		"expires_in":       authResp.ExpiresIn,
		"resume_args":      resumeArgs,
		"hint":             initNoWaitHint,
	}
	encoder := json.NewEncoder(f.IOStreams.Out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to write JSON output: %v", err).WithCause(err)
	}
	return nil
}

// resumeAppRegistration runs the non-blocking second step: poll with a device
// code from a previous --no-wait call, then persist the new app and probe it.
func resumeAppRegistration(opts *ConfigInitOptions) error {
	f := opts.Factory

	rec, err := loadInitNoWaitRecord(opts.DeviceCode)
	if err != nil {
		// The record exists but could not be read/parsed (permissions, disk,
		// corruption). The resume step fully depends on this cache, so surface a
		// storage error instead of the misleading "no pending creation"
		// validation path — the user should fix local storage, not assume the
		// device code is bad and throw away a still-valid creation attempt.
		return errs.NewInternalError(errs.SubtypeStorage, "failed to read the cached resume context: %v", err).WithCause(err)
	}
	if rec == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"no pending app creation found for this device code; re-initiate with `lark-cli config init --new --no-wait`").
			WithParam("--device-code")
	}

	// Expiry check against the cached absolute deadline (device codes are
	// short-lived — the registration default is 300s).
	remaining := rec.ExpiresAt - time.Now().Unix()
	if remaining <= 0 {
		_ = removeInitNoWaitRecord(opts.DeviceCode)
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"device code expired; re-initiate with `lark-cli config init --new --no-wait`").
			WithParam("--device-code")
	}

	// Drift guard (fast path): bail out before the long poll if the config
	// already changed since initiation, so we don't waste minutes polling.
	existing, err := loadConfigForDriftCheck()
	if err != nil {
		return err
	}
	if computeConfigDigest(existing) != rec.ConfigDigest {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"configuration changed since this app creation was started; re-initiate with `lark-cli config init --new --no-wait` to avoid overwriting it").
			WithParam("--device-code")
	}

	interval := rec.Interval
	if interval <= 0 {
		interval = 5
	}

	httpClient := newRegistrationHTTPClient()
	result, pollErr := pollAppRegistrationResume(opts.Ctx, httpClient, opts.DeviceCode, interval, int(remaining), f.IOStreams.ErrOut)
	if pollErr != nil {
		// Clear the cache only on terminal failures (denied / expired /
		// timed-out). Keep it on cancellation or transient errors so the user
		// can retry with the same device code while it is still valid.
		if appRegShouldClearCache(pollErr) {
			_ = removeInitNoWaitRecord(opts.DeviceCode)
		}
		// Pass an already-typed error through unchanged (e.g. the ConfigError
		// for a missing client_id/secret) instead of downgrading it to
		// authentication/unknown — matching runCreateAppFlow.
		if _, ok := errs.ProblemOf(pollErr); ok {
			return pollErr
		}
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "%v", pollErr).WithCause(pollErr)
	}

	// Re-check drift immediately before persisting. The poll above can block
	// for minutes while the user finishes in the browser, and a concurrent
	// process may have changed config.json in that window — saving the stale
	// pre-poll snapshot would drop those edits. Reload and compare again.
	existing, err = loadConfigForDriftCheck()
	if err != nil {
		return err
	}
	if computeConfigDigest(existing) != rec.ConfigDigest {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"configuration changed while the app was being created, so it was not saved (to avoid overwriting that change); re-run `lark-cli config init --new --no-wait`").
			WithParam("--device-code")
	}

	// Determine the final brand from the response, falling back to the cached
	// brand. The cached brand only seeds link generation + this fallback; the
	// Lark-tenant re-poll inside pollAppRegistrationResume is what actually
	// detects a Lark tenant.
	finalBrand := parseBrand(rec.Brand)
	if result.UserInfo != nil && result.UserInfo.TenantBrand == "lark" {
		finalBrand = core.BrandLark
	} else if result.UserInfo != nil && result.UserInfo.TenantBrand == "feishu" {
		finalBrand = core.BrandFeishu
	}

	secret, err := core.ForStorage(result.ClientID, core.PlainSecret(result.ClientSecret), f.Keychain)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "%v", err).WithCause(err)
	}
	if err := saveInitConfig(rec.ProfileName, existing, f, result.ClientID, secret, finalBrand, rec.Lang); err != nil {
		// Preserve a typed error (e.g. the --name conflict ValidationError) via
		// the shared helper instead of downgrading everything to storage —
		// matching the blocking init paths.
		return wrapSaveConfigError(err)
	}

	// Config persisted — only now is it safe to drop the resume cache. Clearing
	// it only after a successful save means a failure in the drift re-check,
	// ForStorage, or saveInitConfig above leaves the cache intact so the user
	// can retry `--device-code` (the remote app already exists).
	_ = removeInitNoWaitRecord(opts.DeviceCode)

	if rec.LangExplicit && rec.Lang != "" {
		msg := getInitMsg(opts.UILang)
		fmt.Fprintln(f.IOStreams.ErrOut, fmt.Sprintf(msg.LangPreferenceSet, rec.Lang))
	}

	output.PrintJson(f.IOStreams.Out, map[string]interface{}{"appId": result.ClientID, "appSecret": "****", "brand": finalBrand})
	if err := runProbe(opts.Ctx, f, result.ClientID, result.ClientSecret, finalBrand); err != nil {
		return err
	}
	return nil
}

// pollAppRegistrationResume polls the registration endpoint (feishu first, then
// the lark endpoint on the tenant_brand=lark special case) and returns the raw
// error so the caller can classify it for cache-cleanup decisions.
func pollAppRegistrationResume(ctx context.Context, httpClient *http.Client, deviceCode string, interval, expiresIn int, errOut io.Writer) (*larkauth.AppRegistrationResult, error) {
	result, err := larkauth.PollAppRegistration(ctx, httpClient, core.BrandFeishu, deviceCode, interval, expiresIn, errOut)
	if err != nil {
		return nil, err
	}
	// Lark tenant special case: if tenant_brand=lark and no client_secret,
	// re-poll against the lark endpoint to obtain the secret.
	if result.ClientSecret == "" && result.UserInfo != nil && result.UserInfo.TenantBrand == "lark" {
		result, err = larkauth.PollAppRegistration(ctx, httpClient, core.BrandLark, deviceCode, interval, expiresIn, errOut)
		if err != nil {
			return nil, err
		}
	}
	if result.ClientID == "" || result.ClientSecret == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidClient, "app registration succeeded but missing client_id or client_secret")
	}
	return result, nil
}

// appRegShouldClearCache reports whether the cached resume context should be
// discarded after a poll outcome. Success and terminal failures (user denied,
// device code expired, deadline elapsed) clear it; cancellation and transient
// errors keep it so the user can retry while the device code is still valid.
func appRegShouldClearCache(err error) bool {
	if err == nil {
		return true
	}
	return errors.Is(err, larkauth.ErrAppRegDenied) ||
		errors.Is(err, larkauth.ErrAppRegExpired) ||
		errors.Is(err, larkauth.ErrAppRegTimeout)
}

// loadConfigForDriftCheck loads the config for the drift comparison. A missing
// config (first-time setup) is fine — it yields a nil config and an empty
// digest. A genuine storage failure (permission denied, corruption) is surfaced
// as a typed storage error rather than being silently read as "config drift".
func loadConfigForDriftCheck() (*core.MultiAppConfig, error) {
	existing, err := core.LoadMultiAppConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "failed to load config for the drift check: %v", err).WithCause(err)
	}
	return existing, nil
}
