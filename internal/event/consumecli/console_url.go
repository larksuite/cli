// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consumecli

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/core"
)

const (
	addonsLandingPath   = "/page/launcher"
	addonsClientIDParam = "clientID"
)

type manifestAddons struct {
	Scopes    *addonsScopes    `json:"scopes,omitempty"`
	Events    *addonsEvents    `json:"events,omitempty"`
	Callbacks *addonsCallbacks `json:"callbacks,omitempty"`
}

type addonsScopes struct {
	Tenant []string `json:"tenant"`
	User   []string `json:"user"`
}

type addonsEvents struct {
	Items addonsEventItems `json:"items"`
}

type addonsEventItems struct {
	Tenant []string `json:"tenant"`
	User   []string `json:"user"`
}

type addonsCallbacks struct {
	Items []string `json:"items"`
}

func encodeAddons(a manifestAddons) (string, error) {
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

func consoleAddonsURL(brand core.LarkBrand, appID string, a manifestAddons) (string, error) {
	encoded, err := encodeAddons(a)
	if err != nil {
		return "", err
	}
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s%s?%s=%s&addons=%s", host, addonsLandingPath, addonsClientIDParam, appID, encoded), nil
}

func consoleLandingURL(brand core.LarkBrand, appID string) string {
	host := core.ResolveEndpoints(brand).Open
	return fmt.Sprintf("%s%s?%s=%s", host, addonsLandingPath, addonsClientIDParam, appID)
}

func addonsHintURL(brand core.LarkBrand, appID string, a manifestAddons) string {
	url, err := consoleAddonsURL(brand, appID, a)
	if err != nil {
		return consoleLandingURL(brand, appID)
	}
	return url
}

func missingScopeAddons(identity core.Identity, missing []string) manifestAddons {
	s := &addonsScopes{Tenant: []string{}, User: []string{}}
	if identity.IsBot() {
		s.Tenant = missing
	} else {
		s.User = missing
	}
	return manifestAddons{Scopes: s}
}

func missingSubscriptionAddons(identity core.Identity, missing []string) manifestAddons {
	ev := &addonsEvents{Items: addonsEventItems{Tenant: []string{}, User: []string{}}}
	if identity.IsBot() {
		ev.Items.Tenant = missing
	} else {
		ev.Items.User = missing
	}
	return manifestAddons{Events: ev}
}
