// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package secplugin implements the ~/.lark-cli/sec_config.json based security proxy plugin mode.
//
// It supports:
// - forcing all outbound HTTP(S) requests through a fixed HTTP proxy
// - trusting an additional root CA PEM bundle for MITM/inspection proxies
// - optional "proxy injects token" mode via placeholder tokens (SEC_AUTH)
//
// In sec plugin mode, certain common CLI env vars (APP_ID / BRAND / DEFAULT_AS /
// STRICT_MODE) can also be set in sec_config.json so sandboxes can avoid
// environment injection. When both are present, environment variables win.
package secplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

// SEC plugin constants cover the config file name and placeholder token values.
const (
	// ConfigFileName is the fixed config file name under core.GetConfigDir().
	ConfigFileName = "sec_config.json"

	// SentinelUAT is the placeholder user access token used in SEC_AUTH mode.
	SentinelUAT = "secplugin-managed-uat"

	// SentinelTAT is the placeholder tenant access token used in SEC_AUTH mode.
	SentinelTAT = "secplugin-managed-tat"
)

// Config is the on-disk config format. Keys intentionally mirror env var names.
type Config struct {
	// Enable turns on sec plugin transport handling.
	Enable bool `json:"LARKSUITE_CLI_SEC_ENABLE"`

	// Proxy is the fixed HTTP proxy address used for all outbound requests.
	Proxy string `json:"LARKSUITE_CLI_SEC_PROXY"`

	// CAPath points to an extra PEM bundle trusted for proxy TLS interception.
	CAPath string `json:"LARKSUITE_CLI_SEC_CA"`

	// Auth enables placeholder-token mode for proxy-side credential injection.
	Auth bool `json:"LARKSUITE_CLI_SEC_AUTH"`

	// Optional defaults for sec plugin mode; env vars override these.
	// AppID supplies the app ID when the environment does not set one.
	AppID string `json:"LARKSUITE_CLI_APP_ID,omitempty"`

	// Brand supplies the tenant brand when the environment does not set one.
	Brand string `json:"LARKSUITE_CLI_BRAND,omitempty"` // feishu | lark

	// DefaultAs supplies the default identity when the environment does not set one.
	DefaultAs string `json:"LARKSUITE_CLI_DEFAULT_AS,omitempty"` // user | bot | auto

	// StrictMode supplies the strict mode when the environment does not set one.
	StrictMode string `json:"LARKSUITE_CLI_STRICT_MODE,omitempty"` // user | bot | off
}

// Path returns the absolute path to the sec plugin config file.
func Path() string {
	return filepath.Join(core.GetConfigDir(), ConfigFileName)
}

// loadOnce guards one-time SEC config loading for process-wide transport reuse.
var loadOnce sync.Once

// loadCfg stores the cached SEC config after the first successful Load call.
var loadCfg *Config

// loadErr stores the cached Load error observed during the first load attempt.
var loadErr error

// Load reads ~/.lark-cli/sec_config.json once and caches the parsed result.
// Environment variables (CliSec*) take precedence over config file values.
//
// Returns (nil, nil) only when:
//   - the config file does not exist AND
//   - none of the SEC-related env vars are present.
func Load() (*Config, error) {
	loadOnce.Do(func() {
		// Start from env-only config if any SEC env var is present.
		cfg, hasEnv, err := loadFromEnv()
		if err != nil {
			loadErr = err
			return
		}

		p := Path()
		if _, err := vfs.Stat(p); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// No file: return env-only config (if any), else nil.
				if hasEnv {
					loadCfg = cfg
				} else {
					loadCfg = nil
				}
				loadErr = nil
				return
			}
			loadErr = fmt.Errorf("failed to stat sec plugin config %q: %w", p, err)
			return
		}
		b, err := vfs.ReadFile(p)
		if err != nil {
			loadErr = fmt.Errorf("failed to read sec plugin config %q: %w", p, err)
			return
		}
		var fileCfg Config
		if err := json.Unmarshal(b, &fileCfg); err != nil {
			loadErr = fmt.Errorf("invalid sec plugin config %q: %w", p, err)
			return
		}

		// Merge: file base + env overrides.
		if cfg == nil {
			cfg = &fileCfg
		} else {
			*cfg = fileCfg
			applyEnvOverrides(cfg)
		}
		loadCfg = cfg
	})
	return loadCfg, loadErr
}

// Enabled reports whether SEC plugin mode is enabled.
func (c *Config) Enabled() bool { return c != nil && c.Enable }

// AuthEnabled reports whether SEC_AUTH token placeholder mode is enabled.
func (c *Config) AuthEnabled() bool { return c != nil && c.Enable && c.Auth }

// loadFromEnv builds a config from SEC-related environment variables only.
// It reports whether any SEC-related environment variable was present.
func loadFromEnv() (*Config, bool, error) {
	_, hasEnable := os.LookupEnv(envvars.CliSecEnable)
	_, hasProxy := os.LookupEnv(envvars.CliSecProxy)
	_, hasCA := os.LookupEnv(envvars.CliSecCA)
	_, hasAuth := os.LookupEnv(envvars.CliSecAuth)
	hasAny := hasEnable || hasProxy || hasCA || hasAuth
	if !hasAny {
		return nil, false, nil
	}
	cfg := &Config{}
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, true, err
	}
	return cfg, true, nil
}

// applyEnvOverrides copies SEC-related environment variable values into cfg.
func applyEnvOverrides(cfg *Config) error {
	if v, ok := os.LookupEnv(envvars.CliSecEnable); ok {
		b, err := parseBoolEnv(envvars.CliSecEnable, v)
		if err != nil {
			return err
		}
		cfg.Enable = b
	}
	if v, ok := os.LookupEnv(envvars.CliSecAuth); ok {
		b, err := parseBoolEnv(envvars.CliSecAuth, v)
		if err != nil {
			return err
		}
		cfg.Auth = b
	}
	if v, ok := os.LookupEnv(envvars.CliSecProxy); ok {
		cfg.Proxy = v
	}
	if v, ok := os.LookupEnv(envvars.CliSecCA); ok {
		cfg.CAPath = v
	}
	return nil
}

// parseBoolEnv accepts common boolean spellings used in environment variables.
func parseBoolEnv(name, raw string) (bool, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		// Treat empty as false when explicitly present.
		return false, nil
	}
	switch s {
	case "1", "true", "on", "yes", "y":
		return true, nil
	case "0", "false", "off", "no", "n":
		return false, nil
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b, nil
	}
	return false, fmt.Errorf("invalid %s %q (want true/false/1/0)", name, raw)
}

// proxyURL validates the fixed SEC proxy configuration and returns its URL.
func (c *Config) proxyURL() (*url.URL, error) {
	raw := strings.TrimSpace(c.Proxy)
	if raw == "" {
		return nil, fmt.Errorf("%s is empty", envvars.CliSecProxy)
	}
	redacted := redactProxyURL(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", envvars.CliSecProxy, redacted, err)
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("invalid %s %q: scheme must be http", envvars.CliSecProxy, redacted)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid %s %q: missing host", envvars.CliSecProxy, redacted)
	}
	// Security hardening: only allow a loopback proxy. This prevents accidental
	// cross-machine proxying of credentials/traffic.
	if u.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("invalid %s %q: host must be 127.0.0.1", envvars.CliSecProxy, redacted)
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("invalid %s %q: explicit port is required", envvars.CliSecProxy, redacted)
	}
	if u.Path != "" && u.Path != "/" {
		return nil, fmt.Errorf("invalid %s %q: path is not allowed", envvars.CliSecProxy, redacted)
	}
	if u.RawQuery != "" {
		return nil, fmt.Errorf("invalid %s %q: query is not allowed", envvars.CliSecProxy, redacted)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("invalid %s %q: fragment is not allowed", envvars.CliSecProxy, redacted)
	}
	return u, nil
}

// redactProxyURL masks userinfo (username:password) in a proxy URL.
// Handles both scheme-prefixed ("http://user:pass@host") and bare formats.
func redactProxyURL(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.User != nil {
		u.User = url.User("***")
		return u.String()
	}
	// Fallback: handle "user:pass@proxy:8080"
	if at := strings.LastIndex(raw, "@"); at > 0 {
		return "***@" + raw[at+1:]
	}
	return raw
}

// ApplyToTransport clones base and applies SEC plugin settings to the clone.
// Caller owns the returned *http.Transport.
func (c *Config) ApplyToTransport(base *http.Transport) (*http.Transport, error) {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport)
	}
	u, err := c.proxyURL()
	if err != nil {
		return nil, err
	}

	t := base.Clone()
	t.Proxy = http.ProxyURL(u) // fixed proxy overrides environment proxy vars
	if err := applyExtraRootCA(t, c.CAPath); err != nil {
		return nil, err
	}
	return t, nil
}
