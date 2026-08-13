// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package externalcredential

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// Mode selects how credentials obtained from an external platform are used by
// the Extended runtime.
type Mode string

const (
	ModePlatformProxy   Mode = "platform_proxy"
	ModeCredentialProxy Mode = "credential_proxy"
	ModeDirect          Mode = "direct"

	defaultCredentialProcessTimeout = 5
	maxCredentialProcessTimeout     = 30
)

func (m Mode) IsProxy() bool {
	return m == ModePlatformProxy || m == ModeCredentialProxy
}

// errUnknownConfigField distinguishes the closed external credential protocol
// from an ordinary malformed Profile. It deliberately remains private to this
// adapter so the general Profile model stays forward compatible.
var errUnknownConfigField = errors.New("unknown external credential config field")

// ProgramConfig pins the administrator-installed executable used by
// credential_proxy and direct modes.
type ProgramConfig struct {
	Executable      string   `json:"executable"`
	Arguments       []string `json:"arguments,omitempty"`
	SHA256          string   `json:"sha256"`
	ProtocolVersion int      `json:"protocolVersion"`
	TimeoutSeconds  int      `json:"timeoutSeconds,omitempty"`
}

// Application identifies a Profile that an administrator allows to use the
// managed runtime.
type Application struct {
	Brand core.LarkBrand `json:"brand"`
	AppID string         `json:"appId"`
}

// Config is loaded from the administrator-controlled
// external-credential.json file, never from a user Profile.
type Config struct {
	Version        int            `json:"version"`
	Mode           Mode           `json:"mode"`
	RemoteEndpoint string         `json:"remoteEndpoint,omitempty"`
	Program        *ProgramConfig `json:"program,omitempty"`
	Applications   []Application  `json:"applications"`
}

func strictUnmarshal(data []byte, dst any) error {
	if err := rejectDuplicateJSONFields(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return fmt.Errorf("%w: %w", errUnknownConfigField, err)
		}
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

// rejectDuplicateJSONFields rejects ambiguous objects before Go's JSON
// decoder can silently apply last-value-wins semantics. It scans recursively
// because the system config and external protocols contain nested policy
// objects whose duplicate fields must be rejected at the same boundary.
func rejectDuplicateJSONFields(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(dec); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func scanJSONValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make([]string, 0)
		for dec.More() {
			nameToken, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return errors.New("JSON object field name must be a string")
			}
			for _, existing := range seen {
				// encoding/json matches struct fields case-insensitively after
				// checking exact names. Apply the same fold here so aliases such
				// as "mode" and "Mode" cannot bypass duplicate rejection.
				if strings.EqualFold(existing, name) {
					return fmt.Errorf("duplicate or case-aliased JSON object field %q", name)
				}
			}
			seen = append(seen, name)
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		closing, err := dec.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("invalid JSON object terminator")
		}
	case '[':
		for dec.More() {
			if err := scanJSONValue(dec); err != nil {
				return err
			}
		}
		closing, err := dec.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("invalid JSON array terminator")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	return strictUnmarshal(data, (*plain)(c))
}

func (c *ProgramConfig) UnmarshalJSON(data []byte) error {
	type plain ProgramConfig
	return strictUnmarshal(data, (*plain)(c))
}

func (a *Application) UnmarshalJSON(data []byte) error {
	type plain Application
	return strictUnmarshal(data, (*plain)(a))
}

// rejectReservedProfileFields prevents a user-controlled Profile from
// masquerading as the administrator-controlled system protocol. Only the
// Profile root and each direct app object are reserved; unrelated future
// payloads remain forward compatible with the ordinary Profile decoder.
func rejectReservedProfileFields(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return errors.New("configuration object must be a JSON object")
	}
	for dec.More() {
		nameToken, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := nameToken.(string)
		if !ok {
			return errors.New("configuration field name must be a string")
		}
		if looksLikeReservedProfileField(name) {
			return fmt.Errorf("%w: json: unknown field %q", errUnknownConfigField, name)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		if name == "apps" {
			if err := scanProfileApps(value); err != nil {
				return err
			}
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func scanProfileApps(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return errors.New("configuration apps must be a JSON array")
	}
	for dec.More() {
		var app json.RawMessage
		if err := dec.Decode(&app); err != nil {
			return err
		}
		if err := scanProfileApp(app); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

func scanProfileApp(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return errors.New("configuration app must be a JSON object")
	}
	for dec.More() {
		nameToken, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := nameToken.(string)
		if !ok {
			return errors.New("configuration field name must be a string")
		}
		if looksLikeReservedProfileField(name) {
			return fmt.Errorf("%w: json: unknown field %q", errUnknownConfigField, name)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}

func looksLikeReservedProfileField(name string) bool {
	var normalized strings.Builder
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' {
			normalized.WriteRune(r)
		}
	}
	return strings.HasPrefix(normalized.String(), "externalcredential")
}

// validateConfig validates the closed system configuration against the
// selected Profile. An enabled configuration never falls back to Profile,
// environment, keychain, OAuth, or compile-time credentials.
func validateConfig(app *core.AppConfig, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	invalidSystemConfig := func(format string, args ...any) error {
		return errs.NewConfigError(errs.SubtypeInvalidConfig, format, args...).
			WithHint("ask the system administrator to repair external-credential.json")
	}
	invalidProfile := func(format string, args ...any) error {
		return errs.NewConfigError(errs.SubtypeInvalidConfig, format, args...).
			WithHint("ask the deploying integrator to repair the selected Profile in config.json")
	}
	invalidApplicationBinding := func(format string, args ...any) error {
		return errs.NewConfigError(errs.SubtypeInvalidConfig, format, args...).
			WithHint("ask the deploying integrator to align the selected Profile in config.json with external-credential.json")
	}
	if cfg.Version != 1 {
		return invalidSystemConfig("external credential configuration has unsupported version %d", cfg.Version)
	}
	if app == nil {
		return invalidProfile("external credential configuration requires a selected Profile")
	}
	if app.AppId == "" {
		return invalidProfile("selected Profile requires appId")
	}
	if app.Brand != core.BrandFeishu && app.Brand != core.BrandLark {
		return invalidProfile("selected Profile has invalid brand %q", app.Brand)
	}
	if !app.AppSecret.IsZero() || len(app.Users) != 0 {
		return invalidProfile("external credential mode cannot be combined with Profile appSecret or users")
	}
	allowed := false
	seen := make(map[string]struct{}, len(cfg.Applications))
	for _, application := range cfg.Applications {
		if application.AppID == "" || (application.Brand != core.BrandFeishu && application.Brand != core.BrandLark) {
			return invalidSystemConfig("external credential configuration contains an invalid application")
		}
		key := string(application.Brand) + "\x00" + application.AppID
		if _, ok := seen[key]; ok {
			return invalidSystemConfig("external credential configuration contains duplicate application %s/%s", application.Brand, application.AppID)
		}
		seen[key] = struct{}{}
		if application.AppID == app.AppId && application.Brand == app.Brand {
			allowed = true
		}
	}
	if len(cfg.Applications) == 0 || !allowed {
		return invalidApplicationBinding("selected Profile application %s/%s is not allowed by external-credential.json", app.Brand, app.AppId)
	}
	validateProgram := func() error {
		if cfg.Program == nil {
			return invalidSystemConfig("%s mode requires program", cfg.Mode)
		}
		if !filepath.IsAbs(cfg.Program.Executable) || strings.ContainsRune(cfg.Program.Executable, 0) {
			return invalidSystemConfig("program.executable must be an absolute path without NUL bytes")
		}
		for _, arg := range cfg.Program.Arguments {
			if strings.ContainsRune(arg, 0) {
				return invalidSystemConfig("program.arguments contains a NUL byte")
			}
		}
		if !strings.HasPrefix(cfg.Program.SHA256, "sha256:") || len(strings.TrimPrefix(cfg.Program.SHA256, "sha256:")) != 64 {
			return invalidSystemConfig("program.sha256 must use sha256:<64 hexadecimal characters>")
		}
		for _, r := range strings.TrimPrefix(cfg.Program.SHA256, "sha256:") {
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return invalidSystemConfig("program.sha256 must use sha256:<64 hexadecimal characters>")
			}
		}
		if cfg.Program.ProtocolVersion != 1 {
			return invalidSystemConfig("program.protocolVersion must be 1")
		}
		if cfg.Program.TimeoutSeconds == 0 {
			cfg.Program.TimeoutSeconds = defaultCredentialProcessTimeout
		}
		if cfg.Program.TimeoutSeconds < 1 || cfg.Program.TimeoutSeconds > maxCredentialProcessTimeout {
			return invalidSystemConfig("program.timeoutSeconds must be between 1 and %d", maxCredentialProcessTimeout)
		}
		return nil
	}
	validateEndpoint := func() error {
		endpoint, err := url.Parse(cfg.RemoteEndpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
			return invalidSystemConfig("%s mode remoteEndpoint must be an HTTPS origin without path, userinfo, query, or fragment", cfg.Mode)
		}
		cfg.RemoteEndpoint = strings.TrimRight(endpoint.String(), "/")
		return nil
	}
	switch cfg.Mode {
	case ModeDirect:
		if cfg.RemoteEndpoint != "" {
			return invalidSystemConfig("direct mode cannot configure remoteEndpoint")
		}
		if err := validateProgram(); err != nil {
			return err
		}
	case ModeCredentialProxy:
		if err := validateEndpoint(); err != nil {
			return err
		}
		if err := validateProgram(); err != nil {
			return err
		}
	case ModePlatformProxy:
		if err := validateEndpoint(); err != nil {
			return err
		}
		if cfg.Program != nil {
			return invalidSystemConfig("platform_proxy mode cannot configure program")
		}
	default:
		return invalidSystemConfig("external credential configuration has unsupported mode %q", cfg.Mode)
	}
	return nil
}
