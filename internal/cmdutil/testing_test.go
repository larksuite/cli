// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/brand"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestTestFactory_ReplacesGlobals(t *testing.T) {
	config := &configpkg.CliConfig{
		AppID: "test-app", AppSecret: "test-secret",
		Brand: brand.Feishu,
	}

	f, stdout, stderr, reg := TestFactory(t, config)

	// Factory should return our config
	got, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error: %v", err)
	}
	if got.AppID != "test-app" {
		t.Errorf("want AppID test-app, got %s", got.AppID)
	}

	// IOStreams.Out/ErrOut should be our buffers
	output.PrintJson(f.IOStreams.Out, map[string]string{"key": "value"})
	if !strings.Contains(stdout.String(), `"key"`) {
		t.Error("output.PrintJson did not write to test stdout")
	}

	output.PrintError(f.IOStreams.ErrOut, "test error")
	if !strings.Contains(stderr.String(), "test error") {
		t.Error("output.PrintError did not write to test stderr")
	}

	// Register a stub so Verify passes
	reg.Register(&httpmock.Stub{
		URL:  "/test",
		Body: "ok",
	})
	// Use the stub via Factory HttpClient.
	httpClient, err := f.HttpClient()
	if err != nil {
		t.Fatalf("HttpClient() error: %v", err)
	}
	baseURL := brand.ResolveOpenBaseURL(brand.Feishu)
	req, _ := http.NewRequest("GET", baseURL+"/test", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("HttpClient request error: %v", err)
	}
	resp.Body.Close()
}
