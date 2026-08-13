// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package externalcredential

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

func TestConfigRejectsUnknownFields(t *testing.T) {
	var cfg Config
	if err := json.Unmarshal([]byte(`{"version":1,"mode":"direct","applications":[],"token":"secret"}`), &cfg); err == nil {
		t.Fatal("expected unknown system configuration field to fail")
	}
	if err := json.Unmarshal([]byte(`{
	  "version":1,
	  "mode":"direct",
	  "program":{
	    "executable":"/bin/helper",
	    "sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	    "protocolVersion":1,
	    "shell":true
	  },
	  "applications":[]
	}`), &cfg); err == nil {
		t.Fatal("expected unknown program field to fail")
	}
	if err := json.Unmarshal([]byte(`{
	  "version":1,
	  "mode":"platform_proxy",
	  "remoteEndpoint":"https://credentials.example",
	  "applications":[{"brand":"feishu","appId":"cli_test","tenant":"secret"}]
	}`), &cfg); err == nil {
		t.Fatal("expected unknown application field to fail")
	}
}

func TestConfigRejectsDuplicateFieldsAtEveryPolicyLevel(t *testing.T) {
	tests := map[string]string{
		"root": `{
		  "version":1,
		  "mode":"direct",
		  "Mode":"platform_proxy",
		  "applications":[]
		}`,
		"program": `{
		  "version":1,
		  "mode":"direct",
		  "program":{
		    "executable":"/bin/helper",
		    "sha256":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		    "SHA256":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		    "protocolVersion":1
		  },
		  "applications":[]
		}`,
		"application": `{
		  "version":1,
		  "mode":"platform_proxy",
		  "remoteEndpoint":"https://credentials.example",
		  "applications":[{"brand":"feishu","appId":"cli_test","AppID":"cli_other"}]
		}`,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal([]byte(data), &cfg); err == nil {
				t.Fatal("expected duplicate external credential configuration field to fail")
			}
		})
	}
}

func TestRejectReservedProfileFields(t *testing.T) {
	tests := []string{
		`{"externalCredential":{"mode":"proxy"},"apps":[]}`,
		`{"apps":[{"appId":"cli_test","brand":"feishu","users":[],"externalCredentials":{"mode":"direct"}}]}`,
		`{"apps":[{"appId":"cli_test","brand":"feishu","users":[],"externalCredential":null}]}`,
		`{"apps":[{"appId":"cli_test","brand":"feishu","users":[],"externalCredential":{"mode":"direct"},"externalCredential":null}]}`,
	}
	for _, data := range tests {
		err := rejectReservedProfileFields([]byte(data))
		if !errors.Is(err, errUnknownConfigField) {
			t.Errorf("data %s: error = %v, want reserved-field error", data, err)
		}
	}
}

func TestReservedProfileScanKeepsFuturePayloadForwardCompatible(t *testing.T) {
	data := []byte(`{
	  "futureFeature":{"externalCredentialInfo":{"enabled":true}},
	  "apps":[{
	    "appId":"cli_test",
	    "brand":"feishu",
	    "users":[],
	    "futureFeature":{"externalCredentialInfo":{"enabled":true}}
	  }]
	}`)
	if err := rejectReservedProfileFields(data); err != nil {
		t.Fatalf("unrelated future payload should remain forward compatible: %v", err)
	}
	var profile core.MultiAppConfig
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("ordinary Profile decoder should remain forward compatible: %v", err)
	}
}

func TestValidateConfigRejectsLegacyCredentialMix(t *testing.T) {
	cfg := validDirectConfig()
	app := &core.AppConfig{
		AppId:     "cli_test",
		AppSecret: core.PlainSecret("secret"),
		Brand:     core.BrandFeishu,
		Users:     []core.AppUser{},
	}
	err := validateConfig(app, &cfg)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("error = %#v, want config/invalid_config", err)
	}
	if problem.Hint != "ask the deploying integrator to repair the selected Profile in config.json" {
		t.Fatalf("hint = %q, want selected Profile recovery", problem.Hint)
	}
}

func TestValidateConfigHintsIdentifyOwningConfiguration(t *testing.T) {
	t.Run("system configuration", func(t *testing.T) {
		cfg := validDirectConfig()
		cfg.Version = 2
		err := validateConfig(validExternalProfile(), &cfg)
		problem, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("error = %T %v, want typed config error", err, err)
		}
		if problem.Hint != "ask the system administrator to repair external-credential.json" {
			t.Fatalf("hint = %q, want system configuration recovery", problem.Hint)
		}
	})

	t.Run("application binding", func(t *testing.T) {
		cfg := validDirectConfig()
		cfg.Applications[0].AppID = "cli_other"
		err := validateConfig(validExternalProfile(), &cfg)
		problem, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("error = %T %v, want typed config error", err, err)
		}
		if !strings.Contains(problem.Hint, "selected Profile in config.json") ||
			!strings.Contains(problem.Hint, "external-credential.json") {
			t.Fatalf("hint = %q, want both configuration owners", problem.Hint)
		}
	})
}

func TestValidateConfigRejectsInvalidEndpoints(t *testing.T) {
	direct := validDirectConfig()
	direct.RemoteEndpoint = "https://proxy.example"
	proxy := validDirectConfig()
	proxy.Mode = ModeCredentialProxy
	proxy.RemoteEndpoint = "http://proxy.example"

	app := validExternalProfile()
	for _, cfg := range []Config{direct, proxy} {
		if err := validateConfig(app, &cfg); err == nil {
			t.Fatalf("expected invalid config to fail: %#v", cfg)
		}
	}
}

func TestValidateConfigSupportsThreeModeShapes(t *testing.T) {
	app := validExternalProfile()
	platform := Config{
		Version:        1,
		Mode:           ModePlatformProxy,
		RemoteEndpoint: "https://proxy.example",
		Applications:   []Application{{Brand: core.BrandFeishu, AppID: "cli_test"}},
	}
	if err := validateConfig(app, &platform); err != nil {
		t.Fatalf("valid platform_proxy: %v", err)
	}

	credentialProxy := validDirectConfig()
	credentialProxy.Mode = ModeCredentialProxy
	credentialProxy.RemoteEndpoint = "https://proxy.example/"
	if err := validateConfig(app, &credentialProxy); err != nil {
		t.Fatalf("valid credential_proxy: %v", err)
	}
	if credentialProxy.RemoteEndpoint != "https://proxy.example" {
		t.Fatalf("normalized endpoint = %q", credentialProxy.RemoteEndpoint)
	}

	direct := validDirectConfig()
	if err := validateConfig(app, &direct); err != nil {
		t.Fatalf("valid direct: %v", err)
	}

	platform.Program = validDirectConfig().Program
	if err := validateConfig(app, &platform); err == nil {
		t.Fatal("platform_proxy accepted a credential program")
	}
	credentialProxy.Program = nil
	if err := validateConfig(app, &credentialProxy); err == nil {
		t.Fatal("credential_proxy accepted a missing credential program")
	}
}

func TestValidateConfigRejectsApplicationOutsideAllowlistAndDuplicates(t *testing.T) {
	app := validExternalProfile()
	notAllowed := validDirectConfig()
	notAllowed.Applications = []Application{{Brand: core.BrandFeishu, AppID: "cli_other"}}
	if err := validateConfig(app, &notAllowed); err == nil {
		t.Fatal("expected selected application outside allowlist to fail")
	}

	duplicate := validDirectConfig()
	duplicate.Applications = append(duplicate.Applications, duplicate.Applications[0])
	if err := validateConfig(app, &duplicate); err == nil {
		t.Fatal("expected duplicate application to fail")
	}
}

func validExternalProfile() *core.AppConfig {
	return &core.AppConfig{
		AppId: "cli_test",
		Brand: core.BrandFeishu,
		Users: []core.AppUser{},
		Lang:  "en-US",
		Name:  "sandbox",
	}
}

func validDirectConfig() Config {
	return Config{
		Version: 1,
		Mode:    ModeDirect,
		Program: &ProgramConfig{
			Executable:      "/bin/helper",
			SHA256:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ProtocolVersion: 1,
			TimeoutSeconds:  5,
		},
		Applications: []Application{{Brand: core.BrandFeishu, AppID: "cli_test"}},
	}
}
