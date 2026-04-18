// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/envvars"
)

type mode uint8

const (
	modeOff mode = iota
	modeWarn
	modeBlock
)

// modeFromEnv reads CliContentSafetyMode. Unknown values write a warning
// to errOut and fall back to modeOff.
func modeFromEnv(errOut io.Writer) mode {
	raw := strings.TrimSpace(os.Getenv(envvars.CliContentSafetyMode))
	if raw == "" {
		return modeOff
	}
	switch strings.ToLower(raw) {
	case "off":
		return modeOff
	case "warn":
		return modeWarn
	case "block":
		return modeBlock
	default:
		fmt.Fprintf(errOut,
			"warning: unknown %s value %q, falling back to off\n",
			envvars.CliContentSafetyMode, raw)
		return modeOff
	}
}

// normalizeCommandPath converts a raw cobra CommandPath() into the dotted
// form used by ALLOWLIST matching.
//
// Rules (applied in order):
//  1. Drop the root command (first segment, e.g. "lark-cli")
//  2. Strip leading "+" from each remaining segment (shortcut marker)
//  3. Replace "-" with "_" inside each segment
//  4. Join segments with "."
func normalizeCommandPath(cobraPath string) string {
	segs := strings.Fields(cobraPath)
	if len(segs) <= 1 {
		return ""
	}
	segs = segs[1:]
	for i, s := range segs {
		s = strings.TrimPrefix(s, "+")
		s = strings.ReplaceAll(s, "-", "_")
		segs[i] = s
	}
	return strings.Join(segs, ".")
}

// isAllowlisted reports whether cmdPath is covered by allowlistEnv.
//
// Rules:
//   - Empty allowlistEnv → false (fail-safe default)
//   - Entry "all" (case-insensitive) → true
//   - Otherwise prefix match: the prefix must equal the full path OR
//     be followed by a literal "."
//   - Entries are compared literally; users must write the exact dotted form
func isAllowlisted(cmdPath, allowlistEnv string) bool {
	if allowlistEnv == "" {
		return false
	}
	for _, entry := range strings.Split(allowlistEnv, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.EqualFold(entry, "all") {
			return true
		}
		if cmdPath == entry || strings.HasPrefix(cmdPath, entry+".") {
			return true
		}
	}
	return false
}

// errBlocked is an internal signal value shared only between
// runContentSafety (producer) and EmitShortcut / EmitLarkResponse
// (consumers, defined in emit.go). Callers of the Emit functions never
// see this value — the Emit functions wrap it into *ExitError before
// returning.
var errBlocked = errors.New("content_safety: response blocked")

// wrapBlockError converts the internal errBlocked signal into a public
// *ExitError with the stable type tag "content_safety_blocked".
func wrapBlockError(alert *extcs.Alert) error {
	return Errorf(
		ExitAPI,
		"content_safety_blocked",
		"response blocked by content safety: %d rule(s) matched",
		len(alert.Matches),
	)
}

// runContentSafety is the sole place where content-safety policy runs.
// Both EmitShortcut and EmitLarkResponse call it as their first step.
//
// Parameters:
//   - cmdPath: the raw cobra CommandPath() string (e.g.
//     "lark-cli im +messages-search"). runContentSafety normalizes it
//     internally before matching ALLOWLIST and before passing the
//     normalized form into ScanRequest.CmdPath. Callers must pass raw,
//     not pre-normalized.
//   - data:    the parsed response payload, unchanged
//   - errOut:  writer for diagnostic warnings (panic / timeout / scan error)
//
// Returns:
//   - (nil, nil) when scanning is off, allowlist misses, no provider
//     is registered, scan times out, or scan panics (fail-open paths)
//   - (alert, nil) when the provider reports a hit in warn mode
//   - (alert, errBlocked) when the provider reports a hit in block mode
func runContentSafety(cmdPath string, data any, errOut io.Writer) (alert *extcs.Alert, err error) {
	mode := modeFromEnv(errOut)
	if mode == modeOff {
		return nil, nil
	}

	normalized := normalizeCommandPath(cmdPath)
	if !isAllowlisted(normalized, os.Getenv(envvars.CliContentSafetyAllowlist)) {
		return nil, nil
	}

	provider := extcs.GetProvider()
	if provider == nil {
		return nil, nil
	}

	scanCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run Scan in a goroutine for two reasons:
	//   1. The deadline is enforced even if the provider ignores ctx.
	//   2. Panic recovery works — recover() only catches panics in the
	//      same goroutine, so the defer must be inside the goroutine.
	// The goroutine may leak if the provider never returns, but that is
	// acceptable: the CLI is a single-command process that exits shortly after.
	type scanResult struct {
		alert *extcs.Alert
		err   error
	}
	ch := make(chan scanResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(errOut,
					"warning: content safety provider %q panicked: %v, passing through\n",
					provider.Name(), r)
				ch <- scanResult{nil, nil}
			}
		}()
		a, scanErr := provider.Scan(scanCtx, extcs.ScanRequest{
			CmdPath: normalized,
			Data:    data,
		})
		ch <- scanResult{a, scanErr}
	}()

	var a *extcs.Alert
	var scanErr error
	select {
	case res := <-ch:
		a, scanErr = res.alert, res.err
	case <-scanCtx.Done():
		fmt.Fprintln(errOut,
			"warning: content safety scan timed out, passing through")
		return nil, nil
	}

	if scanErr != nil {
		fmt.Fprintf(errOut,
			"warning: content safety provider %q returned error: %v, passing through\n",
			provider.Name(), scanErr)
		return nil, nil
	}
	if a == nil {
		return nil, nil
	}
	if mode == modeBlock {
		return a, errBlocked
	}
	return a, nil
}
