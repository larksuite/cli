// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/binding"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

// applyExtraRootCA augments t with an additional PEM bundle used for configured proxy
// TLS interception.
func applyExtraRootCA(t *http.Transport, caPath string) error {
	caPath = strings.TrimSpace(caPath)
	if caPath == "" {
		return nil
	}
	if !filepath.IsAbs(caPath) {
		return fmt.Errorf("invalid %s %q: must be an absolute path to a PEM file", envvars.CliCAPath, caPath)
	}
	safeCAPath, err := binding.AssertSecurePath(binding.AuditParams{
		TargetPath:            caPath,
		Label:                 envvars.CliCAPath,
		AllowReadableByOthers: true,
	})
	if err != nil {
		return fmt.Errorf("unsafe %s %q: %w", envvars.CliCAPath, caPath, err)
	}
	pemBytes, err := vfs.ReadFile(safeCAPath)
	if err != nil {
		return fmt.Errorf("failed to read %s %q: %w", envvars.CliCAPath, caPath, err)
	}

	// Augment the system trust store. Do NOT silently discard a SystemCertPool
	// error: falling back to an empty pool would make this transport trust ONLY
	// the extra CA (dropping all system roots), which narrows trust unexpectedly
	// and could break TLS to legitimate endpoints. Fail closed instead.
	pool, err := x509.SystemCertPool()
	if err != nil {
		return fmt.Errorf("failed to load system cert pool for %s: %w", envvars.CliCAPath, err)
	}
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
		return fmt.Errorf("invalid %s %q: no certificates parsed from PEM", envvars.CliCAPath, caPath)
	}

	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{}
	} else {
		// Clone to avoid mutating shared config from the base transport.
		t.TLSClientConfig = t.TLSClientConfig.Clone()
	}
	if t.TLSClientConfig.MinVersion == 0 || t.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.TLSClientConfig.MinVersion = tls.VersionTLS12
	}
	t.TLSClientConfig.RootCAs = pool
	return nil
}
