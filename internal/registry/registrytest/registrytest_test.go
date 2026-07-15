// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registrytest

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestValidateConfigDir(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name      string
		testRoot  string
		configDir string
		wantErr   bool
	}{
		{name: "equal", testRoot: root, configDir: root},
		{name: "child", testRoot: root, configDir: filepath.Join(root, "config")},
		{
			name:      "sibling",
			testRoot:  root,
			configDir: filepath.Join(filepath.Dir(root), "outside"),
			wantErr:   true,
		},
		{name: "empty root", configDir: root, wantErr: true},
		{name: "empty config", testRoot: root, wantErr: true},
		{name: "relative root", testRoot: "relative", configDir: root, wantErr: true},
		{name: "relative config", testRoot: root, configDir: "relative", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigDir(tt.testRoot, tt.configDir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConfigDir() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFixtureContract(t *testing.T) {
	if len(fixtureMetaJSON) > 20<<10 {
		t.Fatalf("fixture size = %d, want <= %d", len(fixtureMetaJSON), 20<<10)
	}
	reg, err := meta.Parse(fixtureMetaJSON)
	if err != nil {
		t.Fatalf("meta.Parse() error = %v", err)
	}
	if reg.Version != "0.0.1" {
		t.Fatalf("fixture version = %q, want 0.0.1", reg.Version)
	}

	gotNames := make([]string, 0, len(reg.Services))
	for _, service := range reg.Services {
		gotNames = append(gotNames, service.Name)
	}
	sort.Strings(gotNames)
	if !slices.Equal(gotNames, []string{"calendar", "im", "task"}) {
		t.Fatalf("fixture services = %v, want [calendar im task]", gotNames)
	}

	calendarCreate := fixtureMethod(t, reg, "calendar", "events", "create")
	assertMethodContract(t, calendarCreate, "calendars/{calendar_id}/events", http.MethodPost)
	calendarID, ok := calendarCreate.Parameters["calendar_id"]
	if !ok || calendarID.Location != "path" || !calendarID.Required {
		t.Fatalf("calendar_id = %+v, want required path parameter", calendarID)
	}
	if !slices.Contains(calendarCreate.Scopes, "calendar:calendar.event:create") {
		t.Fatalf("calendar create scopes = %v, want calendar:calendar.event:create", calendarCreate.Scopes)
	}

	imCreate := fixtureMethod(t, reg, "im", "chat.members", "create")
	assertMethodContract(t, imCreate, "chats/{chat_id}/members", http.MethodPost)
	chatID, ok := imCreate.Parameters["chat_id"]
	if !ok || chatID.Location != "path" || !chatID.Required {
		t.Fatalf("chat_id = %+v, want required path parameter", chatID)
	}
	memberIDType, ok := imCreate.Parameters["member_id_type"]
	if !ok || memberIDType.Location != "query" || memberIDType.Required {
		t.Fatalf("member_id_type = %+v, want optional query parameter", memberIDType)
	}
	if imCreate.Risk != "write" {
		t.Fatalf("im create risk = %q, want write", imCreate.Risk)
	}
	for _, scope := range []string{"im:chat", "im:chat.members:write_only"} {
		if !slices.Contains(imCreate.Scopes, scope) {
			t.Fatalf("im create scopes = %v, want %s", imCreate.Scopes, scope)
		}
	}
}

func fixtureMethod(t *testing.T, reg meta.Registry, serviceName, resourceName, methodName string) meta.Method {
	t.Helper()
	for _, service := range reg.Services {
		if service.Name != serviceName {
			continue
		}
		resource, ok := service.Resource(resourceName)
		if !ok {
			t.Fatalf("fixture service %s has no resource %s", serviceName, resourceName)
		}
		method, ok := resource.Method(methodName)
		if !ok {
			t.Fatalf("fixture resource %s.%s has no method %s", serviceName, resourceName, methodName)
		}
		return method
	}
	t.Fatalf("fixture has no service %s", serviceName)
	return meta.Method{}
}

func assertMethodContract(t *testing.T, method meta.Method, path, httpMethod string) {
	t.Helper()
	if method.Path != path || method.HTTPMethod != httpMethod {
		t.Fatalf("method = %s %s, want %s %s", method.HTTPMethod, method.Path, httpMethod, path)
	}
}

// TestSeedRejectsUnsafeConfigDir pins Seed's guard: it must return before
// writing anything when LARKSUITE_CLI_CONFIG_DIR is unset or escapes the
// caller's test root, so a TestMain wiring mistake can never touch a
// developer's real ~/.lark-cli.
func TestSeedRejectsUnsafeConfigDir(t *testing.T) {
	root := t.TempDir()

	t.Run("unset config dir", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")
		if err := Seed(root); err == nil {
			t.Fatal("Seed() error = nil, want unset config dir rejection")
		}
	})

	t.Run("config dir outside test root", func(t *testing.T) {
		outside := t.TempDir()
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", outside)
		if err := Seed(root); err == nil {
			t.Fatal("Seed() error = nil, want containment rejection")
		}
		if _, err := os.Stat(filepath.Join(outside, "cache")); err == nil {
			t.Fatal("Seed wrote into the rejected config dir")
		}
	})
}
