// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package transport owns how the CLI assembles its outbound HTTP transport: the
// shared base RoundTripper (Shared/Fallback/NewHTTPClient), the LARK_CLI_NO_PROXY
// direct-egress clone, and the ~/.lark-cli/proxy_config.json proxy-plugin mode.
//
// Proxy-plugin mode forces all outbound HTTP(S) requests through a fixed loopback
// proxy, optionally trusting an extra root CA PEM bundle for TLS-inspection
// proxies, and fails closed on misconfiguration. Environment variables override
// matching values from proxy_config.json.
package transport

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

	"github.com/larksuite/cli/internal/binding"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

// ConfigFileName is the fixed config file name under core.GetConfigDir().
const (
	ConfigFileName = "proxy_config.json"
)

// Config is the on-disk config format. Keys intentionally mirror env var names.
type Config struct {
	// Enable turns on proxy plugin transport handling.
	Enable bool `json:"LARKSUITE_CLI_PROXY_ENABLE"`

	// Proxy is the fixed HTTP proxy address used for all outbound requests.
	Proxy string `json:"LARKSUITE_CLI_PROXY_ADDRESS"`

	// CAPath points to an extra PEM bundle trusted for proxy TLS interception.
	CAPath string `json:"LARKSUITE_CLI_CA_PATH"`
}

// Path returns the absolute path to the proxy plugin config file.
func Path() string {
	return filepath.Join(core.GetConfigDir(), ConfigFileName)
}

// loadOnce guards one-time proxy config loading for process-wide transport reuse.
var loadOnce sync.Once

// loadCfg stores the cached proxy config after the first successful Load call.
var loadCfg *Config

// loadErr stores the cached Load error observed during the first load attempt.
var loadErr error

// Load reads ~/.lark-cli/proxy_config.json once and caches the parsed result.
// Environment variables (CliProxyEnable/CliProxyAddress/CliCAPath) take precedence over config file values.
//
// Returns (nil, nil) only when:
//   - the config file does not exist AND
//   - none of the proxy-related env vars are present.
func Load() (*Config, error) {
	loadOnce.Do(func() {
		// Start from env-only config if any proxy env var is present.
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
			loadErr = fmt.Errorf("failed to stat proxy plugin config %q: %w", p, err)
			return
		}
		// Security hardening: this config dictates where ALL outbound CLI traffic
		// egresses and which extra CA is trusted, so a file another local user or
		// process can tamper with (symlink, foreign owner, group/world-writable)
		// could redirect credential traffic. Audit it the same way the CA file is.
		safePath, err := binding.AssertSecurePath(binding.AuditParams{
			TargetPath:            p,
			Label:                 ConfigFileName,
			AllowReadableByOthers: true, // config is not a secret; only writability/owner/symlink matter
		})
		if err != nil {
			loadErr = fmt.Errorf("unsafe proxy plugin config %q: %w", p, err)
			return
		}
		b, err := vfs.ReadFile(safePath)
		if err != nil {
			loadErr = fmt.Errorf("failed to read proxy plugin config %q: %w", p, err)
			return
		}
		var fileCfg Config
		if err := json.Unmarshal(b, &fileCfg); err != nil {
			loadErr = fmt.Errorf("invalid proxy plugin config %q: %w", p, err)
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

// Enabled reports whether proxy plugin mode is enabled.
func (c *Config) Enabled() bool { return c != nil && c.Enable }

// loadFromEnv builds a config from proxy-related environment variables only.
// It reports whether any proxy-related environment variable was present.
func loadFromEnv() (*Config, bool, error) {
	_, hasEnable := os.LookupEnv(envvars.CliProxyEnable)
	_, hasProxy := os.LookupEnv(envvars.CliProxyAddress)
	_, hasCA := os.LookupEnv(envvars.CliCAPath)
	hasAny := hasEnable || hasProxy || hasCA
	if !hasAny {
		return nil, false, nil
	}
	cfg := &Config{}
	if err := applyEnvOverrides(cfg); err != nil {
		return nil, true, err
	}
	return cfg, true, nil
}

// applyEnvOverrides copies proxy-related environment variable values into cfg.
func applyEnvOverrides(cfg *Config) error {
	if v, ok := os.LookupEnv(envvars.CliProxyEnable); ok {
		b, err := parseBoolEnv(envvars.CliProxyEnable, v)
		if err != nil {
			return err
		}
		cfg.Enable = b
	}
	if v, ok := os.LookupEnv(envvars.CliProxyAddress); ok {
		cfg.Proxy = v
	}
	if v, ok := os.LookupEnv(envvars.CliCAPath); ok {
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

// proxyURL validates the fixed configured proxy configuration and returns its URL.
func (c *Config) proxyURL() (*url.URL, error) {
	raw := strings.TrimSpace(c.Proxy)
	if raw == "" {
		return nil, fmt.Errorf("%s is empty", envvars.CliProxyAddress)
	}
	redacted := redactProxyURL(raw)
	u, err := url.Parse(raw)
	if err != nil {
		// Do not wrap the raw url.Parse error: its string embeds the original
		// URL, which can contain userinfo (user:password). Return a redacted,
		// generic message instead.
		return nil, fmt.Errorf("invalid %s %q: malformed URL", envvars.CliProxyAddress, redacted)
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("invalid %s %q: scheme must be http", envvars.CliProxyAddress, redacted)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid %s %q: missing host", envvars.CliProxyAddress, redacted)
	}
	// Security hardening: only allow a loopback proxy. This prevents accidental
	// cross-machine proxying of credentials/traffic.
	if u.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("invalid %s %q: host must be 127.0.0.1", envvars.CliProxyAddress, redacted)
	}
	if u.Port() == "" {
		return nil, fmt.Errorf("invalid %s %q: explicit port is required", envvars.CliProxyAddress, redacted)
	}
	if u.Path != "" {
		return nil, fmt.Errorf("invalid %s %q: path is not allowed", envvars.CliProxyAddress, redacted)
	}
	if u.RawQuery != "" {
		return nil, fmt.Errorf("invalid %s %q: query is not allowed", envvars.CliProxyAddress, redacted)
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("invalid %s %q: fragment is not allowed", envvars.CliProxyAddress, redacted)
	}
	return u, nil
}

// ApplyToTransport clones base and applies proxy plugin settings to the clone.
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
