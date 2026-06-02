// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package secplugin

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

// applyExtraRootCA augments t with an additional PEM bundle used for SEC proxy
// TLS interception.
func applyExtraRootCA(t *http.Transport, caPath string) error {
	caPath = strings.TrimSpace(caPath)
	if caPath == "" {
		return nil
	}
	if !filepath.IsAbs(caPath) {
		return fmt.Errorf("invalid %s %q: must be an absolute path to a PEM file", envvars.CliSecCA, caPath)
	}
	pemBytes, err := vfs.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("failed to read %s %q: %w", envvars.CliSecCA, caPath, err)
	}

	// Start from system pool when possible; if unavailable, create a new pool.
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
		return fmt.Errorf("invalid %s %q: no certificates parsed from PEM", envvars.CliSecCA, caPath)
	}

	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{}
	} else {
		// Clone to avoid mutating shared config from the base transport.
		t.TLSClientConfig = t.TLSClientConfig.Clone()
	}
	t.TLSClientConfig.MinVersion = tls.VersionTLS12
	t.TLSClientConfig.RootCAs = pool
	return nil
}
