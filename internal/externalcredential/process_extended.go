// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

// Package externalcredential implements the runtime contract between lark-cli
// and an executable supplied by an external credential platform.
package externalcredential

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
)

const (
	protocolVersion = 1
	maxOutputBytes  = 64 << 10
	refreshWindow   = 60 * time.Second

	// Proxy credentials cross the sandbox trust boundary. Keep their lifetime
	// deliberately short even if a credential process is misconfigured.
	maxProxyCredentialTTL = time.Hour
	processWaitDelay      = time.Second
)

var errCredentialProcessTimeout = errors.New("external credential process timeout")

const (
	credentialAccessToken = "access_token"
	credentialProxyToken  = "proxy_access_token"

	proxyUserPlaceholder = "lark-cli-proxy-user-v1"
	proxyBotPlaceholder  = "lark-cli-proxy-bot-v1"
)

// Request is written as one JSON value to the credential process stdin.
type Request struct {
	Version        int    `json:"version"`
	Mode           Mode   `json:"mode"`
	CredentialType string `json:"credential_type,omitempty"`
	AppID          string `json:"app_id"`
	Brand          string `json:"brand"`
	Identity       string `json:"identity,omitempty"`
	RemoteEndpoint string `json:"remote_endpoint,omitempty"`
}

type response struct {
	Version    int             `json:"version"`
	Credential *wireCredential `json:"credential,omitempty"`
	Error      *wireError      `json:"error,omitempty"`
}

type wireCredential struct {
	TokenType   string   `json:"token_type,omitempty"`
	Scheme      string   `json:"scheme,omitempty"`
	AccessToken string   `json:"access_token,omitempty"`
	ExpiresAt   string   `json:"expires_at,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type cacheKey struct {
	credentialType string
	identity       string
}

type cacheEntry struct {
	credential wireCredential
	expiresAt  time.Time
}

// Provider is a fail-closed credential source backed by the selected Profile
// and administrator-controlled system configuration.
type Provider struct {
	profile *core.AppConfig
	config  *Config
	now     func() time.Time

	mu       sync.Mutex
	cache    map[cacheKey]cacheEntry
	inflight map[cacheKey]*call

	// processEnvironment is a test seam. Production always uses the explicit
	// credentialProcessEnvironment allowlist installed by NewProvider.
	processEnvironment func([]string) ([]string, error)
}

type call struct {
	done chan struct{}
	cred wireCredential
	err  error
}

// NewProvider creates a provider for an already validated profile.
func NewProvider(app *core.AppConfig, config *Config) *Provider {
	return &Provider{
		profile:            app,
		config:             config,
		now:                time.Now,
		cache:              make(map[cacheKey]cacheEntry),
		inflight:           make(map[cacheKey]*call),
		processEnvironment: credentialProcessEnvironment,
	}
}

func (p *Provider) Name() string { return "external-credential-platform" }

// SkipUserInfoEnrichment prevents account discovery from making an OpenAPI
// request before the proxy transport is fully installed.
func (p *Provider) SkipUserInfoEnrichment() bool { return true }

// CredentialCapabilities exposes only source-neutral inspection behavior.
func (p *Provider) CredentialCapabilities() credential.ProviderCapabilities {
	return credential.ProviderCapabilities{
		SkipUserInfoEnrichment: true,
		ProvidesOnDemandAuth:   true,
		CanInspectScopes:       p != nil && p.config != nil && p.config.Mode == ModeDirect,
	}
}

// CredentialAccountMetadata preserves ordinary Profile preferences while the
// managed credential protocol remains private to this adapter.
func (p *Provider) CredentialAccountMetadata() credential.ProviderAccountMetadata {
	if p == nil || p.profile == nil {
		return credential.ProviderAccountMetadata{}
	}
	return credential.ProviderAccountMetadata{Lang: p.profile.Lang}
}

func (p *Provider) ResolveAccount(context.Context) (*extcred.Account, error) {
	if p == nil || p.profile == nil || p.config == nil {
		return nil, nil
	}
	if name := firstConfiguredEnvironment(credentialSourceEnvironmentNames()); name != "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"system external credential mode cannot be combined with credential or identity environment variable %s", name).
			WithHint("remove the legacy credential or identity environment variable before using external-credential.json")
	}
	if p.config.Mode.IsProxy() {
		if name := firstConfiguredEnvironment(localTransportEnvironmentNames()); name != "" {
			return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
				"external credential proxy mode cannot be combined with local transport environment variable %s", name).
				WithHint("remove the local proxy or CA environment variable when using a managed proxy mode")
		}
	}
	return &extcred.Account{
		AppID:               p.profile.AppId,
		Brand:               extcred.Brand(p.profile.Brand),
		DefaultAs:           extcred.Identity(p.profile.DefaultAs),
		ProfileName:         p.profile.ProfileName(),
		SupportedIdentities: extcred.SupportsAll,
	}, nil
}

func (p *Provider) ResolveToken(ctx context.Context, spec extcred.TokenSpec) (*extcred.Token, error) {
	if spec.AppID != p.profile.AppId {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"external credential request app_id %q does not match profile app_id %q", spec.AppID, p.profile.AppId)
	}
	identity := string(core.AsUser)
	wantType := string(extcred.TokenTypeUAT)
	if spec.Type == extcred.TokenTypeTAT {
		identity = string(core.AsBot)
		wantType = string(extcred.TokenTypeTAT)
	} else if spec.Type != extcred.TokenTypeUAT {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported external credential token type %q", spec.Type)
	}
	if p.config.Mode != ModeDirect {
		placeholder := proxyUserPlaceholder
		if identity == string(core.AsBot) {
			placeholder = proxyBotPlaceholder
		}
		return &extcred.Token{Value: placeholder, Source: p.Name()}, nil
	}
	cred, err := p.resolve(ctx, cacheKey{credentialType: credentialAccessToken, identity: identity})
	if err != nil {
		return nil, err
	}
	if p.config.Mode == ModeDirect && cred.TokenType != wantType {
		return nil, invalidResponse("external credential process returned token_type %q for identity %q", cred.TokenType, identity)
	}
	return &extcred.Token{
		Value:  cred.AccessToken,
		Scopes: strings.Join(cred.Scopes, " "),
		Source: p.Name(),
	}, nil
}

// ResolveProxyCredential returns the short-lived bearer used only on the
// credential_proxy wire. Ordinary SDK token resolution receives a non-secret
// placeholder so a proxy credential cannot accidentally enter a direct
// Feishu request before the Transport rewrites it.
func (p *Provider) ResolveProxyCredential(ctx context.Context, identity core.Identity) (string, error) {
	if p.config.Mode != ModeCredentialProxy {
		return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"proxy access credentials are only available in credential_proxy mode")
	}
	cred, err := p.resolve(ctx, cacheKey{credentialType: credentialProxyToken, identity: string(identity)})
	if err != nil {
		return "", err
	}
	return cred.AccessToken, nil
}

func (p *Provider) resolve(ctx context.Context, key cacheKey) (wireCredential, error) {
	p.mu.Lock()
	if entry, ok := p.cache[key]; ok && (entry.expiresAt.IsZero() || p.now().Add(refreshWindow).Before(entry.expiresAt)) {
		p.mu.Unlock()
		return entry.credential, nil
	}
	if pending, ok := p.inflight[key]; ok {
		p.mu.Unlock()
		select {
		case <-pending.done:
			return pending.cred, pending.err
		case <-ctx.Done():
			return wireCredential{}, ctx.Err()
		}
	}
	pending := &call{done: make(chan struct{})}
	p.inflight[key] = pending
	p.mu.Unlock()

	cred, expiry, err := p.execute(ctx, key)
	if problem, ok := errs.ProblemOf(err); ok && problem.Category != errs.CategoryConfig {
		err = errs.WithDiagnosticMetadata(err, errs.DiagnosticMetadata{Origin: "credential_process"})
	}
	p.mu.Lock()
	if err == nil {
		p.cache[key] = cacheEntry{credential: cred, expiresAt: expiry}
	}
	pending.cred, pending.err = cred, err
	delete(p.inflight, key)
	close(pending.done)
	p.mu.Unlock()
	return cred, err
}

func (p *Provider) execute(ctx context.Context, key cacheKey) (wireCredential, time.Time, error) {
	req := Request{
		Version:        protocolVersion,
		Mode:           p.config.Mode,
		AppID:          p.profile.AppId,
		Brand:          string(p.profile.Brand),
		Identity:       key.identity,
		RemoteEndpoint: p.config.RemoteEndpoint,
	}
	if p.config.Mode == ModeDirect {
		req.CredentialType = key.credentialType
	} else if p.config.Mode == ModeCredentialProxy {
		req.CredentialType = credentialProxyToken
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return wireCredential{}, time.Time{}, invalidResponse("cannot encode external credential request")
	}
	if err := verifyCredentialProgram(p.config.Program); err != nil {
		return wireCredential{}, time.Time{}, err
	}
	timeout := time.Duration(p.config.Program.TimeoutSeconds) * time.Second
	procCtx, cancel := context.WithTimeoutCause(ctx, timeout, errCredentialProcessTimeout)
	defer cancel()
	environment := p.processEnvironment
	if environment == nil {
		environment = credentialProcessEnvironment
	}
	processEnv, envErr := environment(os.Environ())
	if envErr != nil {
		return wireCredential{}, time.Time{}, isolatedProcessEnvironmentError(envErr)
	}
	cmd, cmdErr := newCredentialProcessCommand(procCtx, p.config.Program, payload, processEnv)
	if cmdErr != nil {
		return wireCredential{}, time.Time{}, isolatedProcessEnvironmentError(cmdErr)
	}
	// Bound the time spent waiting for inherited stdout/stderr pipes after the
	// credential process exits or is killed. Without WaitDelay, a descendant
	// that keeps a pipe open can make Cmd.Run wait indefinitely.
	cmd.WaitDelay = processWaitDelay
	var stdout limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	err = cmd.Run()
	if err != nil && errors.Is(context.Cause(procCtx), errCredentialProcessTimeout) {
		return wireCredential{}, time.Time{}, errs.NewNetworkError(errs.SubtypeCredentialSourceUnavailable, "external credential process timed out after %s", timeout).WithCause(procCtx.Err()).WithRetryable()
	}
	if err != nil && ctx.Err() != nil {
		return wireCredential{}, time.Time{}, ctx.Err()
	}
	if stdout.tooLarge {
		return wireCredential{}, time.Time{}, invalidResponse("external credential process response exceeds %d bytes", maxOutputBytes)
	}
	resp, decodeErr := decodeResponse(stdout.Bytes())
	if decodeErr == nil {
		if envelopeErr := validateResponseEnvelope(resp); envelopeErr != nil {
			return wireCredential{}, time.Time{}, envelopeErr
		}
	}
	if err != nil {
		if decodeErr == nil && resp.Error != nil {
			return wireCredential{}, time.Time{}, classifyProcessError(resp.Error)
		}
		if decodeErr == nil {
			return wireCredential{}, time.Time{}, invalidResponse("external credential process failure response must contain error only")
		}
		return wireCredential{}, time.Time{}, errs.NewInternalError(errs.SubtypeExternalTool,
			"external credential process exited unsuccessfully").
			WithCause(err).
			WithHint("the helper runs with no caller environment or PATH; use an absolute native executable and express dependencies through administrator-controlled arguments or files")
	}
	if decodeErr != nil {
		return wireCredential{}, time.Time{}, decodeErr
	}
	return validateResponse(resp, p.config.Mode, p.now())
}

type limitedBuffer struct {
	bytes.Buffer
	tooLarge bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := maxOutputBytes + 1 - b.Len()
	if remaining <= 0 {
		b.tooLarge = true
		return original, nil
	}
	if len(p) > remaining {
		p = p[:remaining]
		b.tooLarge = true
	}
	_, _ = b.Buffer.Write(p)
	return original, nil
}

func decodeResponse(data []byte) (*response, error) {
	if err := rejectDuplicateJSONFields(data); err != nil {
		return nil, invalidResponse("external credential process returned ambiguous JSON: %v", err)
	}
	var resp response
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&resp); err != nil {
		return nil, invalidResponse("external credential process returned invalid JSON: %v", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, invalidResponse("external credential process returned more than one JSON value")
	}
	return &resp, nil
}

func validateResponse(resp *response, mode Mode, now time.Time) (wireCredential, time.Time, error) {
	if err := validateResponseEnvelope(resp); err != nil {
		return wireCredential{}, time.Time{}, err
	}
	if resp.Error != nil {
		return wireCredential{}, time.Time{}, invalidResponse("external credential process success response must contain credential only")
	}
	c := *resp.Credential
	if c.AccessToken == "" || c.ExpiresAt == "" {
		return wireCredential{}, time.Time{}, invalidResponse("external credential process returned an invalid access credential")
	}
	expiresAt, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil || !expiresAt.After(now.Add(refreshWindow)) {
		return wireCredential{}, time.Time{}, invalidResponse("external credential process credential must expire more than 60 seconds in the future")
	}
	if mode == ModeDirect {
		if c.Scheme != "" || (c.TokenType != "uat" && c.TokenType != "tat") {
			return wireCredential{}, time.Time{}, invalidResponse("direct mode requires token_type uat or tat")
		}
	} else if mode != ModeCredentialProxy || c.Scheme != "bearer" || c.TokenType != "" || len(c.Scopes) != 0 {
		return wireCredential{}, time.Time{}, invalidResponse("credential_proxy mode requires a bearer proxy credential")
	} else if expiresAt.After(now.Add(maxProxyCredentialTTL)) {
		return wireCredential{}, time.Time{}, invalidResponse("proxy credential lifetime must not exceed %s", maxProxyCredentialTTL)
	}
	return c, expiresAt, nil
}

func validateResponseEnvelope(resp *response) error {
	if resp == nil {
		return invalidResponse("external credential process returned an empty response")
	}
	if resp.Version != protocolVersion {
		return invalidResponse("external credential process returned unsupported version %d", resp.Version)
	}
	if (resp.Credential == nil) == (resp.Error == nil) {
		return invalidResponse("external credential process response must contain exactly one of credential or error")
	}
	return nil
}

func classifyProcessError(e *wireError) error {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "external credential process failed"
	}
	switch e.Code {
	case "temporarily_unavailable":
		return errs.NewNetworkError(errs.SubtypeCredentialSourceUnavailable, "%s", message).WithRetryable()
	case "access_denied":
		return errs.NewSecurityPolicyError(errs.SubtypeAccessDenied, "%s", message)
	case "invalid_request", "unsupported_identity":
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", message)
	default:
		return errs.NewInternalError(errs.SubtypeExternalTool, "%s", message)
	}
}

func invalidResponse(format string, args ...any) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...)
}

func isolatedProcessEnvironmentError(err error) error {
	return errs.NewInternalError(errs.SubtypeExternalTool,
		"cannot construct an isolated environment for the external credential process").
		WithCause(err).
		WithHint("verify the operating system can resolve its trusted runtime directory; caller environment values are never used as a fallback")
}

func newCredentialProcessCommand(
	ctx context.Context,
	program *ProgramConfig,
	payload []byte,
	environment []string,
) (*exec.Cmd, error) {
	if environment == nil {
		return nil, errors.New("credential process environment must be explicit")
	}
	if runtime.GOOS == "windows" && !hasNonEmptyEnvironmentValue(environment, "SYSTEMROOT") {
		// os/exec otherwise fills SYSTEMROOT from the parent process. Reject the
		// command before construction so a failed trusted-directory lookup can
		// never restore caller-controlled loader state.
		return nil, errors.New("trusted Windows SYSTEMROOT is unavailable")
	}
	cmd := exec.CommandContext(ctx, program.Executable, program.Arguments...)
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	cmd.Env = environment
	// The executable and each ancestor have already passed the platform trust
	// check. Using its canonical parent gives helpers a stable working directory
	// without exposing them to the caller's potentially writable cwd.
	cmd.Dir = filepath.Dir(filepath.Clean(program.Executable))
	return cmd, nil
}

func credentialProcessEnvironment(env []string) ([]string, error) {
	// Do not inherit caller-controlled proxy, CA, loader, runtime, locale, temp,
	// HOME, or PATH settings. Administrators must express helper dependencies
	// through the pinned executable/configuration instead of ambient process
	// state. The platform helper supplies only OS values obtained independently
	// of env; env remains a parameter solely for the test seam.
	_ = env
	trusted, err := trustedCredentialProcessEnvironment()
	if err != nil {
		return nil, err
	}
	if trusted == nil {
		// exec.Cmd treats nil Env as "inherit the parent environment".
		return []string{}, nil
	}
	return trusted, nil
}

func hasNonEmptyEnvironmentValue(environment []string, want string) bool {
	for _, item := range environment {
		name, value, ok := strings.Cut(item, "=")
		if ok && strings.EqualFold(name, want) && value != "" {
			return true
		}
	}
	return false
}

func firstConfiguredEnvironment(names []string) string {
	for _, name := range names {
		if os.Getenv(name) != "" {
			return name
		}
	}
	return ""
}

func credentialSourceEnvironmentNames() []string {
	return []string{
		envvars.CliAppID, envvars.CliAppSecret, envvars.CliBrand,
		envvars.CliUserAccessToken, envvars.CliTenantAccessToken,
		envvars.CliDefaultAs, envvars.CliStrictMode,
		envvars.CliAuthProxy, envvars.CliProxyKey,
	}
}

func localTransportEnvironmentNames() []string {
	return []string{
		envvars.CliProxyEnable, envvars.CliProxyAddress, envvars.CliCAPath,
	}
}

var _ extcred.Provider = (*Provider)(nil)
