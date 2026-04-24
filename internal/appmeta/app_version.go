// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package appmeta exposes read-only views of a Feishu app's self-declared
// metadata (published version, subscribed event types, requested scopes).
// It's intentionally a thin wrapper over the Open API so callers can make
// preflight decisions without each one re-parsing the same response shape.
package appmeta

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/event"
)

// APIClient is the minimal surface appmeta needs from the CLI's HTTP stack.
// Aliased to event.APIClient so all three narrow interfaces (event,
// appmeta, consume) are one and the same type — a concrete adapter
// implements the shape once and satisfies all call sites. Callers
// typically pass a bot/TAT-pinned adapter because /app_versions rejects
// UAT with 99991668.
type APIClient = event.APIClient

// AppVersion is the projected subset of one /app_versions item that preflight
// checks care about. Fields we don't consume (audit metadata, visibility,
// avatars) are dropped to keep the struct small and the contract obvious.
type AppVersion struct {
	VersionID    string
	Version      string
	EventTypes   []string // subscribed event types, e.g. "im.message.receive_v1"
	TenantScopes []string // scopes whose token_types contains "tenant"
}

// appVersionStatusPublished is the integer value the OAPI emits for a version
// that has passed audit (and, paired with a non-empty publish_time, is live).
// Documented values: 0=unknown, 1=audit-passed, 2=audit-rejected, 3=under-
// audit, 4=not-submitted.
const appVersionStatusPublished = 1

// FetchCurrentPublished returns the most recently published version of appID,
// or (nil, nil) when the app has never been published. It propagates API
// errors verbatim so callers can decide whether to treat them as hard
// failures or weak-dependency skips.
//
// Why page_size=2 is enough: Feishu disallows creating a new version while
// any in-progress version (status ∈ {2, 3, 4}) exists, so at any moment the
// items array sorted by create_time desc is [<maybe one in-progress>,
// <latest published>, ...]. The first item with status==1 and non-empty
// publish_time in the first two items is therefore always the live one.
func FetchCurrentPublished(ctx context.Context, client APIClient, appID string) (*AppVersion, error) {
	path := fmt.Sprintf(
		"/open-apis/application/v6/applications/%s/app_versions?lang=zh_cn&page_size=2",
		appID,
	)
	raw, err := client.CallAPI(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Data struct {
			Items []struct {
				VersionID   string          `json:"version_id"`
				Version     string          `json:"version"`
				Status      int             `json:"status"`
				PublishTime json.RawMessage `json:"publish_time"`
				EventInfos  []struct {
					EventType string `json:"event_type"`
				} `json:"event_infos"`
				Scopes []struct {
					Scope      string   `json:"scope"`
					TokenTypes []string `json:"token_types"`
				} `json:"scopes"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode app_versions response: %w", err)
	}

	for _, it := range envelope.Data.Items {
		if it.Status != appVersionStatusPublished || !publishTimeSet(it.PublishTime) {
			continue
		}
		v := &AppVersion{
			VersionID: it.VersionID,
			Version:   it.Version,
		}
		for _, e := range it.EventInfos {
			if e.EventType != "" {
				v.EventTypes = append(v.EventTypes, e.EventType)
			}
		}
		for _, s := range it.Scopes {
			if s.Scope != "" && containsString(s.TokenTypes, "tenant") {
				v.TenantScopes = append(v.TenantScopes, s.Scope)
			}
		}
		return v, nil
	}
	return nil, nil
}

// publishTimeSet accepts the two shapes the OAPI has been observed to emit
// for an unpublished item: JSON null and the empty string. Any other value
// is taken as a real publish_time (we don't parse the number — just its
// presence).
func publishTimeSet(raw json.RawMessage) bool {
	s := string(raw)
	return s != "" && s != "null" && s != `""`
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
