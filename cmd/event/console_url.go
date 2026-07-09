// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/core"
	eventlib "github.com/larksuite/cli/internal/event"
)

// Landing-page contract for the scan-to-enable deep link, verified against the
// open platform: {open-host}/page/launcher?clientID=<appID>&addons=<encoded>.
// Note the param is camelCase "clientID" (not snake_case), and the value is the
// consuming app's own ID. Centralized so it can be corrected in one place.
const (
	addonsLandingPath   = "/page/launcher"
	addonsClientIDParam = "clientID"
)

// ManifestAddons mirrors the 5 public manifest sections the launcher page accepts.
// Encoded form: JSON -> gzip -> base64url(no padding).
type ManifestAddons struct {
	Scopes    *AddonsScopes    `json:"scopes,omitempty"`
	Events    *AddonsEvents    `json:"events,omitempty"`
	Callbacks *AddonsCallbacks `json:"callbacks,omitempty"`
}

type AddonsScopes struct {
	Tenant []string `json:"tenant"`
	User   []string `json:"user"`
}

type AddonsEvents struct {
	Items AddonsEventItems `json:"items"`
}

type AddonsEventItems struct {
	Tenant []string `json:"tenant"`
	User   []string `json:"user"`
}

type AddonsCallbacks struct {
	Items []string `json:"items"`
}

// encodeAddons: JSON -> gzip -> base64url(no padding). Matches the front-end decode chain.
func encodeAddons(a ManifestAddons) (string, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(raw); err != nil {
		return "", err
	}
	if err := gw.Close(); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// consoleAddonsURL builds the scan-to-enable deep link carrying incremental scopes/events/callbacks.
func consoleAddonsURL(brand core.LarkBrand, appID string, a ManifestAddons) (string, error) {
	encoded, err := encodeAddons(a)
	if err != nil {
		return "", err
	}
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s%s?%s=%s&addons=%s", host, addonsLandingPath, addonsClientIDParam, appID, encoded), nil
}

// consoleLandingURL is the bare landing page (no addons) — fallback when encoding fails.
func consoleLandingURL(brand core.LarkBrand, appID string) string {
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s%s?%s=%s", host, addonsLandingPath, addonsClientIDParam, appID)
}

// addonsHintURL returns the scan URL, degrading to the bare landing page on encode error.
func addonsHintURL(brand core.LarkBrand, appID string, a ManifestAddons) string {
	url, err := consoleAddonsURL(brand, appID, a)
	if err != nil {
		return consoleLandingURL(brand, appID)
	}
	return url
}

// missingScopeAddons routes missing scopes into the identity-appropriate section.
// The unused side is an empty (non-nil) slice so JSON encodes [] not null —
// the addons spec treats a missing tenant/user as an empty array.
func missingScopeAddons(identity core.Identity, missing []string) ManifestAddons {
	s := &AddonsScopes{Tenant: []string{}, User: []string{}}
	if identity.IsBot() {
		s.Tenant = missing
	} else {
		s.User = missing
	}
	return ManifestAddons{Scopes: s}
}

// missingSubscriptionAddons routes missing events/callbacks into the right section.
// Like missingScopeAddons, unused event sides stay [] (not null) per the addons spec.
func missingSubscriptionAddons(subType eventlib.SubscriptionType, identity core.Identity, missing []string) ManifestAddons {
	if subType == eventlib.SubTypeCallback {
		return ManifestAddons{Callbacks: &AddonsCallbacks{Items: missing}}
	}
	ev := &AddonsEvents{Items: AddonsEventItems{Tenant: []string{}, User: []string{}}}
	if identity.IsBot() {
		ev.Items.Tenant = missing
	} else {
		ev.Items.User = missing
	}
	return ManifestAddons{Events: ev}
}
