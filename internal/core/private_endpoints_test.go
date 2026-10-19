package core

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPrivateServiceBasesDoNotFallBackToPublicCloud(t *testing.T) {
	cfg := &CliConfig{Brand: BrandFeishu, APIBaseURL: "https://api.tenant.test/prefix", AuthorizationBaseURL: "https://login.tenant.test", AuthProtocol: "legacy_oidc"}
	ep, err := cfg.ResolveServiceEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	if ep.Open != "https://api.tenant.test/prefix" || ep.Accounts != "https://login.tenant.test" {
		t.Fatalf("wrong service bases: %#v", ep)
	}
	cfg.APIBaseURL = "https://user:secret@api.tenant.test/?token=secret"
	if _, err := cfg.ResolveServiceEndpoints(); err == nil {
		t.Fatal("invalid private base must fail closed")
	}
}

func TestConcurrentAuthUsersMergeWithoutChangingOtherApps(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := SaveMultiAppConfig(&MultiAppConfig{Apps: []AppConfig{{AppId: "first", Brand: BrandFeishu}, {AppId: "selected", Brand: BrandFeishu}}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"alice", "bob"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := MergeAppUser("selected", AppUser{UserOpenId: id}); err != nil {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	cfg, err := LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Apps[0].Users) != 0 || len(cfg.Apps[1].Users) != 2 {
		t.Fatalf("app/user records lost: %#v", cfg)
	}
}

func TestIncompletePrivateBasesAndMalformedLegacyDomainFailClosed(t *testing.T) {
	for _, cfg := range []*CliConfig{{Brand: LarkBrand("https://")}, {Brand: BrandFeishu, APIBaseURL: "https://api.tenant.test"}, {Brand: BrandFeishu, AuthorizationBaseURL: "https://login.tenant.test"}} {
		if _, err := cfg.ResolveServiceEndpoints(); err == nil {
			t.Fatalf("incomplete private endpoint silently fell back: %#v", cfg)
		}
	}
}

func TestAuthConfigMergePreservesUnknownFieldsAtEveryLevel(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := os.WriteFile(GetConfigPath(), []byte(`{"futureRoot":1,"apps":[{"appId":"app","futureApp":2,"users":[{"userOpenId":"alice","userName":"old","futureUser":3}]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MergeAppUser("app", AppUser{UserOpenId: "alice", UserName: "new"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(GetConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"futureRoot", "futureApp", "futureUser"} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("unknown configuration field erased: %s", key)
		}
	}
}
