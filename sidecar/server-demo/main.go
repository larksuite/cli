// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar_demo

// Command sidecar-server-demo is a reference implementation of a sidecar
// auth proxy server. It is NOT production-ready — integrators should
// implement their own server conforming to the wire protocol defined in
// github.com/larksuite/cli/sidecar.
//
// The demo reuses the lark-cli credential pipeline (keychain + config) to
// resolve real tokens, so it only works on a machine that has been
// configured with `lark-cli auth login`.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/sidecar"
)

func main() {
	listen := flag.String("listen", sidecar.DefaultListenAddr, "listen address (host:port)")
	keyFile := flag.String("key-file", defaultKeyFile(), "path to write the HMAC key")
	logFile := flag.String("log-file", "", "audit log file (stderr if empty)")
	profile := flag.String("profile", "", "lark-cli profile name (empty = active profile)")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, *listen, *keyFile, *logFile, *profile); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func defaultKeyFile() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".lark-sidecar", "proxy.key")
	}
	return "/tmp/lark-sidecar/proxy.key"
}

func run(ctx context.Context, listen, keyFile, logFile, profile string) error {
	// Reject self-proxy: if this process inherited AUTH_PROXY, the sidecar
	// credential provider would activate and return sentinel tokens instead
	// of real ones, breaking the "trusted side holds real credentials" premise.
	if v := os.Getenv(envvars.CliAuthProxy); v != "" {
		return fmt.Errorf("%s is set in this environment (%s); unset it before starting the sidecar server", envvars.CliAuthProxy, v)
	}
	if listen == "" {
		return fmt.Errorf("invalid --listen address: empty")
	}

	// Generate HMAC key (32 bytes = 256 bits) and write it to disk (0600).
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return fmt.Errorf("failed to generate HMAC key: %v", err)
	}
	keyHex := hex.EncodeToString(keyBytes)

	keyDir := filepath.Dir(keyFile)
	if err := vfs.MkdirAll(keyDir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %v", err)
	}
	if err := vfs.WriteFile(keyFile, []byte(keyHex), 0600); err != nil {
		return fmt.Errorf("failed to write key file: %v", err)
	}

	// Audit logger: file or stderr.
	var auditLogger *log.Logger
	if logFile != "" {
		f, err := vfs.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open log file: %v", err)
		}
		defer f.Close()
		auditLogger = log.New(f, "", log.LstdFlags)
	} else {
		auditLogger = log.New(os.Stderr, "[audit] ", log.LstdFlags)
	}

	// Reuse the lark-cli credential pipeline. A production implementation
	// would likely source credentials from a secrets manager instead.
	factory := cmdutil.NewDefault(nil, cmdutil.InvocationContext{Profile: profile})
	cfg, err := factory.Config()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", listen, err)
	}
	defer listener.Close()

	allowedHosts := buildAllowedHosts(
		core.ResolveEndpoints(core.BrandFeishu),
		core.ResolveEndpoints(core.BrandLark),
	)
	allowedIDs := buildAllowedIdentities(cfg)

	handler := &proxyHandler{
		key:          []byte(keyHex),
		cred:         factory.Credential,
		appID:        cfg.AppID,
		brand:        cfg.Brand,
		logger:       auditLogger,
		forwardCl:    newForwardClient(),
		allowedHosts: allowedHosts,
		allowedIDs:   allowedIDs,
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		auditLogger.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			auditLogger.Printf("shutdown error: %v", err)
		}
	}()

	keyPrefix := keyHex
	if len(keyPrefix) > 8 {
		keyPrefix = keyPrefix[:8]
	}
	proxyURL := "http://" + listen
	fmt.Fprintf(os.Stderr, "Auth sidecar listening on %s\n", proxyURL)
	fmt.Fprintf(os.Stderr, "HMAC key prefix: %s\n", keyPrefix)
	fmt.Fprintf(os.Stderr, "Full key written to %s (mode 0600)\n", keyFile)
	fmt.Fprintf(os.Stderr, "\nSet in sandbox:\n")
	fmt.Fprintf(os.Stderr, "  export %s=%q\n", envvars.CliAuthProxy, proxyURL)
	fmt.Fprintf(os.Stderr, "  export %s=\"<read from %s>\"\n", envvars.CliProxyKey, keyFile)
	fmt.Fprintf(os.Stderr, "  export %s=%q\n", envvars.CliAppID, cfg.AppID)
	fmt.Fprintf(os.Stderr, "  export %s=%q\n", envvars.CliBrand, string(cfg.Brand))

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("sidecar server exited unexpectedly: %v", err)
	}
	return nil
}
