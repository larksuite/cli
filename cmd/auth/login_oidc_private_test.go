package auth

import (
	"net/url"
	"testing"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestLoginOIDCProtocolFlagReachesLegacyURLWithoutConfigMutation(t *testing.T) {
	cfg := &core.CliConfig{AppID: "app", AppSecret: "synthetic", Brand: core.LarkBrand("https://open.tenant.test")}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	command := NewCmdAuthLoginOIDC(f, func(opts *OIDCLoginOptions) error {
		flow, err := oidcLoginFlowOptions(opts, cfg)
		if err != nil {
			return err
		}
		built, err := larkauth.BuildOIDCAuthorizationURL(flow, "state")
		if err != nil {
			return err
		}
		parsed, _ := url.Parse(built)
		if parsed.Path != "/open-apis/authen/v1/index" || flow.AuthProtocol != "legacy_oidc" || flow.OpenDomain != "https://open.tenant.test" || flow.AppID != "app" || len(parsed.Query()) != 3 {
			t.Fatalf("flag did not reach legacy flow: %s", built)
		}
		if cfg.AuthProtocol != "" {
			t.Fatal("per-invocation protocol override mutated config")
		}
		return nil
	})
	command.SetArgs([]string{"--auth-protocol", "legacy_oidc"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
}
