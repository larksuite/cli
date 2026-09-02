// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package render

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/events"
	"github.com/larksuite/cli/internal/event/catalog"
)

// This file is a guard, not a contract: it does not pin what the redaction
// regex matches, it hunts for declared parameter names that smell like
// credentials yet would render verbatim. The detector wordlist is therefore
// deliberately wider than the production sensitiveParamName pattern — a hit
// here means either the parameter should be renamed or the production
// pattern must grow, decided by a human, never by loosening this list.

// credentialWords are matched against whole '_'/'-'/'.'-separated segments of
// a parameter name, so chat_key or tokenizer_mode cannot trip them. The bare
// word "key" is intentionally absent (identifier names like whiteboard_id or
// a hypothetical chat_key are not credentials); the api/key pairing is what
// carries credential semantics and is detected as a pair below.
var credentialWords = map[string]bool{
	"token":       true,
	"secret":      true,
	"password":    true,
	"credential":  true,
	"credentials": true,
	"cookie":      true,
	"auth":        true,
	"signature":   true,
	"bearer":      true,
	"apikey":      true,
}

// smellsLikeCredential reports whether a parameter name carries credential
// semantics per the guard wordlist: any single segment in credentialWords,
// or the adjacent segment pair api+key.
func smellsLikeCredential(name string) bool {
	segments := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	for i, seg := range segments {
		if credentialWords[seg] {
			return true
		}
		if seg == "api" && i+1 < len(segments) && segments[i+1] == "key" {
			return true
		}
	}
	return false
}

// unredactedCredentialParams returns the names that smell like credentials
// but are NOT matched by the production redaction pattern — every such name
// would render its value verbatim in a dry-run decision.
func unredactedCredentialParams(names []string) []string {
	var findings []string
	for _, name := range names {
		if smellsLikeCredential(name) && !sensitiveParamName.MatchString(name) {
			findings = append(findings, name)
		}
	}
	return findings
}

// The detector itself must bite before the live scan means anything: known
// credential-shaped names that the production pattern misses must be caught,
// and ordinary identifier names must pass.
func TestRedactionGuardDetector_SelfCheck(t *testing.T) {
	// Credential-shaped and covered by the production pattern: no finding.
	for _, name := range []string{"access_token", "client_secret", "user_password", "session_cookie", "sso_credential"} {
		if got := unredactedCredentialParams([]string{name}); len(got) != 0 {
			t.Errorf("%q is redacted by the production pattern, the guard must not flag it, got %v", name, got)
		}
	}
	// Credential-shaped but NOT covered by the production pattern today: the
	// guard must flag these, otherwise it can never catch a real gap.
	for _, name := range []string{"api_key", "auth_code", "request_signature", "bearer_value"} {
		if got := unredactedCredentialParams([]string{name}); len(got) != 1 {
			t.Errorf("%q smells like a credential and is not redacted; the guard must flag it, got %v", name, got)
		}
	}
	// Ordinary identifiers, including the wide-false-positive shapes the
	// wordlist is segment-matched to avoid: no finding.
	for _, name := range []string{"whiteboard_id", "chat_key", "tokenizer_mode", "author", "meeting_no"} {
		if got := unredactedCredentialParams([]string{name}); len(got) != 0 {
			t.Errorf("%q is an ordinary identifier, the guard must not flag it, got %v", name, got)
		}
	}
}

// Every declared parameter of every compiled EventKey either carries no
// credential semantics or is caught by the production redaction pattern.
func TestRedactionGuard_CatalogParamsHaveNoUnredactedCredentials(t *testing.T) {
	snap, err := catalog.Compile(events.All(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("compile catalog: %v", err)
	}

	var names []string
	for _, entry := range snap.Entries() {
		desc := entry.Descriptor()
		for _, p := range desc.Params {
			names = append(names, desc.Key+": "+p.Name)
			if findings := unredactedCredentialParams([]string{p.Name}); len(findings) != 0 {
				t.Errorf("EventKey %s declares param %q which smells like a credential but is not matched by the redaction pattern; rename the param or extend sensitiveParamName deliberately", desc.Key, p.Name)
			}
		}
	}
	// A scan that visited no parameters proves nothing.
	if len(names) == 0 {
		t.Fatal("the compiled catalog declares no parameters at all; the guard scanned nothing")
	}
}
