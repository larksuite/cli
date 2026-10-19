package doc

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func TestReadCommandExplicitDocumentHostUsesOnlyConfiguredAPI(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "separate-host-tracer", AppSecret: "synthetic", Brand: core.LarkBrand("https://open.tenant.test")})
	reg.Register(&httpmock.Stub{URL: "https://open.tenant.test/open-apis/auth/v3/tenant_access_token/internal", Body: map[string]interface{}{"code": 0, "tenant_access_token": "synthetic", "expire": 7200}})
	reg.Register(&httpmock.Stub{URL: "https://open.tenant.test/open-apis/docx/v1/documents/doc1", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}}})
	reg.Register(&httpmock.Stub{URL: "https://open.tenant.test/open-apis/docx/v1/documents/doc1/raw_content", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"content": "body"}}})
	parent := &cobra.Command{Use: "docs"}
	DocsRead.Mount(parent, f)
	parent.SetArgs([]string{"+read", "--doc", "https://documents.tenant.test/docx/doc1", "--document-host", "documents.tenant.test"})
	if err := parent.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || !strings.Contains(stdout.String(), `"content": "body"`) {
		t.Fatalf("read failed: %s", stdout.String())
	}
}

func TestReadCommandDeniedBeforeContentDoesNotEmitSuccessfulStdout(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "read-denied-no-success", AppSecret: "synthetic", Brand: core.BrandFeishu})
	reg.Register(&httpmock.Stub{URL: "/open-apis/auth/v3/tenant_access_token/internal", Body: map[string]interface{}{"code": 0, "tenant_access_token": "synthetic", "expire": 7200}})
	for i := 0; i < 2; i++ {
		reg.Register(&httpmock.Stub{URL: "/open-apis/docx/v1/documents/doc", Status: 403, Body: map[string]interface{}{"code": 1770032, "msg": "forbidden"}})
	}
	parent := &cobra.Command{Use: "docs"}
	DocsRead.Mount(parent, f)
	parent.SetArgs([]string{"+read", "--doc", "doc"})
	if err := parent.Execute(); err == nil {
		t.Fatal("denied read accepted")
	}
	if stdout.Len() != 0 {
		t.Fatalf("no material read but stdout claims success: %s", stdout.String())
	}
}

func TestReadCommandPartialMaterialIsExplicitlyNotOK(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "read-material-partial", AppSecret: "synthetic", Brand: core.BrandFeishu})
	reg.Register(&httpmock.Stub{URL: "/open-apis/auth/v3/tenant_access_token/internal", Body: map[string]interface{}{"code": 0, "tenant_access_token": "synthetic", "expire": 7200}})
	document := map[string]interface{}{"code": 0, "data": map[string]interface{}{"document": map[string]interface{}{"revision_id": 1}}}
	reg.Register(&httpmock.Stub{URL: "/open-apis/docx/v1/documents/doc", Body: document})
	reg.Register(&httpmock.Stub{URL: "/open-apis/docx/v1/documents/doc/raw_content", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"content": "retained-body"}}})
	reg.Register(&httpmock.Stub{URL: "/open-apis/docx/v1/documents/doc/blocks", Status: 403, Body: map[string]interface{}{"code": 1770032}})
	reg.Register(&httpmock.Stub{URL: "/open-apis/docx/v1/documents/doc", Body: document})
	parent := &cobra.Command{Use: "docs"}
	DocsRead.Mount(parent, f)
	parent.SetArgs([]string{"+read", "--doc", "doc", "--full"})
	if err := parent.Execute(); err == nil {
		t.Fatal("incomplete material accepted")
	}
	if !strings.Contains(stdout.String(), `"ok": false`) || !strings.Contains(stdout.String(), `"partial": true`) || !strings.Contains(stdout.String(), "retained-body") || strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("partial contract missing: %s", stdout.String())
	}
}
