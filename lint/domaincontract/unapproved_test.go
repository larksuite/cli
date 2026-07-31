// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"testing"
)

func scanDomainEvidence(t *testing.T, source string) []domainEvidence {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v\n%s", err, source)
	}
	scan := newFileDomainScan(typedGoFile{File: file, Fset: fset})
	scan.collectSemanticEvidence()
	scan.collectAbsoluteURLEvidence()
	sort.Slice(scan.Evidence, func(i, j int) bool {
		if scan.Evidence[i].Host != scan.Evidence[j].Host {
			return scan.Evidence[i].Host < scan.Evidence[j].Host
		}
		return scan.Evidence[i].Expr.Pos() < scan.Evidence[j].Expr.Pos()
	})
	return scan.Evidence
}

func scanTypedDomainEvidence(t *testing.T, source string) []domainEvidence {
	t.Helper()
	return scanTypedDomainEvidenceInPackage(t, "fixture", source)
}

func scanTypedDomainEvidenceInPackage(t *testing.T, packagePath, source string) []domainEvidence {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", source, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v\n%s", err, source)
	}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	if _, err := (&types.Config{}).Check(packagePath, fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("type-check fixture: %v\n%s", err, source)
	}
	scan := newFileDomainScan(typedGoFile{File: file, Fset: fset, Info: info})
	scan.collectSemanticEvidence()
	scan.collectAbsoluteURLEvidence()
	sort.Slice(scan.Evidence, func(i, j int) bool {
		if scan.Evidence[i].Host != scan.Evidence[j].Host {
			return scan.Evidence[i].Host < scan.Evidence[j].Host
		}
		return scan.Evidence[i].Expr.Pos() < scan.Evidence[j].Expr.Pos()
	})
	return scan.Evidence
}

func evidenceHosts(evidence []domainEvidence) []string {
	hosts := make([]string, 0, len(evidence))
	for _, item := range evidence {
		hosts = append(hosts, item.Host)
	}
	return hosts
}

func TestTypedAbsoluteURLDeclarationProducesOneFinding(t *testing.T) {
	evidence := scanTypedDomainEvidence(t,
		"package p\nconst DomainContractE2EURL = \"https://private.corp.internal/v1\"\n")
	if got := evidenceHosts(evidence); len(got) != 1 || got[0] != "private.corp.internal" {
		t.Fatalf("hosts = %v, want [private.corp.internal]", got)
	}
}

func TestGoDomainEvidenceTruePositives(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "PR 1975 Feishu assignment",
			source: "package p\nfunc f() { host := \"internal-api-drive-stream.feishu.cn\"; _ = host }\n",
			want:   []string{"internal-api-drive-stream.feishu.cn"},
		},
		{
			name:   "PR 1975 Lark assignment",
			source: "package p\nfunc f() { var host string; host = \"internal-api-drive-stream.larksuite.com\"; _ = host }\n",
			want:   []string{"internal-api-drive-stream.larksuite.com"},
		},
		{
			name:   "uppercase snake target",
			source: "package p\nfunc f() { API_HOST := \"private.corp.internal\"; _ = API_HOST }\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "typed declaration",
			source: "package p\nconst APIHost string = \"attacker.zip\"\n",
			want:   []string{"attacker.zip"},
		},
		{
			name:   "grouped const declaration",
			source: "package p\nconst (\n APIHost string = \"attacker.zip\"\n)\n",
			want:   []string{"attacker.zip"},
		},
		{
			name:   "grouped var declaration",
			source: "package p\nvar (\n APIHost string = \"attacker.zip\"\n)\n",
			want:   []string{"attacker.zip"},
		},
		{
			name: "multi assignment",
			source: "package p\nfunc f() {\n" +
				" APIHost, BackupHost := \"public.example.com\", \"attacker.zip\"\n" +
				" _, _ = APIHost, BackupHost\n}\n",
			want: []string{"attacker.zip", "public.example.com"},
		},
		{
			name:   "map semantic key",
			source: "package p\nvar c = map[string]string{\"host\": \"private.corp.internal\"}\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "map semantic key assignment",
			source: "package p\nfunc f() { c := map[string]string{}; c[\"host\"] = \"private.corp.internal\" }\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "host collection values",
			source: "package p\nvar ALLOWED_HOSTS = []string{\"private.corp.internal\", \"attacker.zip\"}\n",
			want:   []string{"attacker.zip", "private.corp.internal"},
		},
		{
			name:   "host collection map keys",
			source: "package p\nvar allowedHosts = map[string]struct{}{\"attacker.zip\": {}}\n",
			want:   []string{"attacker.zip"},
		},
		{
			name:   "host collection bool map keys",
			source: "package p\nvar AllowedHosts = map[string]bool{\"api.example.com\": true}\n",
			want:   []string{"api.example.com"},
		},
		{
			name:   "host collection map values",
			source: "package p\nvar HostsByRegion = map[string]string{\"sg\": \"api.example.com\"}\n",
			want:   []string{"api.example.com"},
		},
		{
			name: "host collection map value assignment",
			source: "package p\nfunc f() {\n" +
				" HostsByRegion := map[string]string{}\n" +
				" HostsByRegion[\"sg\"] = \"api.example.com\"\n" +
				"}\n",
			want: []string{"api.example.com"},
		},
		{
			name:   "static concatenation",
			source: "package p\nvar APIHost = \"attacker.\" + \"zip\"\n",
			want:   []string{"attacker.zip"},
		},
		{
			name:   "multiline assignment",
			source: "package p\nfunc f() {\n APIHost :=\n  \"attacker.zip\"\n _ = APIHost\n}\n",
			want:   []string{"attacker.zip"},
		},
		{
			name:   "escaped hostname",
			source: "package p\nvar APIHost = \"private\\u002ecorp\\u002einternal\"\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "hex escaped hostname",
			source: "package p\nvar APIHost = \"private\\x2ecorp\\x2einternal\"\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "octal escaped hostname",
			source: "package p\nvar APIHost = \"private\\056corp\\056internal\"\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "raw hostname",
			source: "package p\nvar APIHost = `private.corp.internal`\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name: "same-file constant reference",
			source: "package p\nconst existingConst = \"private.corp.internal\"\n" +
				"func f() { APIHost := existingConst; _ = APIHost }\n",
			want: []string{"private.corp.internal"},
		},
		{
			name:   "absolute URL",
			source: "package p\nvar message = \"https://private.corp.internal/v1\"\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "websocket URL with port",
			source: "package p\nvar endpoint = \"wss://private.corp.internal:443/v1\"\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "URL userinfo query and fragment",
			source: "package p\nvar endpoint = \" https://user:pass@private.corp.internal:8443/v1?q=1#result \"\n",
			want:   []string{"private.corp.internal"},
		},
		{
			name:   "IDN hostname",
			source: "package p\nvar APIHost = \"例子.公司.cn\"\n",
			want:   []string{"例子.公司.cn"},
		},
		{
			name:   "case port and trailing dot normalization",
			source: "package p\nvar APIHost = \"EXAMPLE.COM.:443\"\n",
			want:   []string{"example.com"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := evidenceHosts(scanDomainEvidence(t, tc.source))
			if len(got) != len(tc.want) {
				t.Fatalf("hosts = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("hosts = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestGoDomainEvidenceTrueNegatives(t *testing.T) {
	source := `package p

import _ "github.com/larksuite/oapi-sdk-go/v3"

var file = "archive.zip"
var event = "card.action.trigger"
var schema = "im.messages.list"
var configFile = "service.prod.json"
var version = "v1.2.3"
var email = "name@example.com"
var lowConfidence = "attacker.zip"
var downloadURL = "archive.zip/file"
var prose = "See https://private.corp.internal/v1 for details"
// https://private.corp.internal/v1
var ghost = "private.corp.internal"
var hostnameParser = "private.corp.internal"
var domainError = "private.corp.internal"
var APIHost = "localhost"
var BackupHost = "127.0.0.1"
var hosts = struct{ File string }{File: "archive.zip"}
var AllowedHosts = map[string]string{"api.example.com": "client.pem"}

func dynamicValue() string { return "private.corp.internal" }
var DynamicHost = dynamicValue()

func setAmbiguousHostMetadata() {
	AllowedHosts["api.example.com"] = "client.pem"
}
`
	if got := scanDomainEvidence(t, source); len(got) != 0 {
		t.Fatalf("unexpected evidence: %+v", got)
	}
}

func TestTypedStructFieldHostnameSemantics(t *testing.T) {
	t.Run("network fields", func(t *testing.T) {
		source := `package source

type Config struct { Host string }
type FeishuSource struct { Domain string }

var config = Config{Host: "api.example.com"}
var source = FeishuSource{Domain: "events.example.com"}
`
		got := evidenceHosts(scanTypedDomainEvidenceInPackage(
			t,
			"github.com/larksuite/cli/internal/event/source",
			source,
		))
		want := []string{"api.example.com", "events.example.com"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("hosts = %v, want %v", got, want)
		}
	})

	t.Run("command metadata domain", func(t *testing.T) {
		source := `package cmdmeta

type Meta struct { Domain string }

var meta = Meta{Domain: "im.messages"}
func update(meta *Meta) { meta.Domain = "docs.pages" }
`
		if got := scanTypedDomainEvidenceInPackage(
			t,
			"github.com/larksuite/cli/internal/cmdmeta",
			source,
		); len(got) != 0 {
			t.Fatalf("unexpected command metadata evidence: %+v", got)
		}
	})

	t.Run("card action host", func(t *testing.T) {
		source := `package im

type CardActionTriggerOutput struct { Host string }

var output = CardActionTriggerOutput{Host: "card.action"}
func update(output *CardActionTriggerOutput) { output.Host = "im.message" }
`
		if got := scanTypedDomainEvidenceInPackage(
			t,
			"github.com/larksuite/cli/events/im",
			source,
		); len(got) != 0 {
			t.Fatalf("unexpected card host evidence: %+v", got)
		}
	})

	t.Run("unknown field ownership is conservative", func(t *testing.T) {
		source := "package p\ntype Config struct { Host string }\nvar c = Config{Host: \"api.example.com\"}\n"
		if got := scanDomainEvidence(t, source); len(got) != 0 {
			t.Fatalf("unexpected untyped field evidence: %+v", got)
		}
	})
}

func TestHostnameSemanticNames(t *testing.T) {
	for _, name := range []string{
		"host", "HOST", "hosts", "hostname", "domains",
		"api_host", "API_HOST", "ALLOWED_HOSTS",
		"apiHost", "APIHost", "backupHostname",
		"HostsByRegion", "APIHostsByRegion", "hostsByRegion",
	} {
		if !isHostnameSemanticName(name) {
			t.Errorf("%q should be hostname-semantic", name)
		}
	}
	for _, name := range []string{
		"ghost", "hostnameParser", "domainError", "hostValue", "downloadURL", "endpoint", "origin",
		"HostBypass", "APIHostBypass",
	} {
		if isHostnameSemanticName(name) {
			t.Errorf("%q must not be hostname-semantic", name)
		}
	}
}

func TestDomainFixturePaths(t *testing.T) {
	for _, path := range []string{
		"internal/x/x_test.go",
		"tests/cli_e2e/x.go",
		"internal/x/testdata/sample.go",
	} {
		if !isDomainFixturePath(path) {
			t.Errorf("%q should be fixture scope", path)
		}
	}
	for _, path := range []string{
		"internal/x/test_helper.go",
		"examples/demo.go",
		"skills/example/testdata/sample.go",
		"skills/example/example_test.go",
	} {
		if isDomainFixturePath(path) {
			t.Errorf("%q must not be fixture scope", path)
		}
	}
}
