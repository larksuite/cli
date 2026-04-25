// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package appmeta exposes read-only views of a Feishu app's published version, subscribed event types, and scopes.
package appmeta

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/event"
)

// APIClient aliases event.APIClient so one concrete adapter satisfies event, appmeta, and consume.
type APIClient = event.APIClient

// AppVersion is the projected subset of one /app_versions item preflight cares about.
type AppVersion struct {
	VersionID    string
	Version      string
	EventTypes   []string
	TenantScopes []string
}

const appVersionStatusPublished = 1

// FetchCurrentPublished returns the most recently published version of appID, or (nil, nil) if never published.
// page_size=2 suffices: Feishu disallows a new version while an in-progress one exists, so the first status==1 item with publish_time is the live one.
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

// publishTimeSet rejects null and empty-string; any other value is a real publish_time.
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
