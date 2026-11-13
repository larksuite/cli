// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

func decodeSearchReport(t *testing.T, out *bytes.Buffer) searchReport {
	t.Helper()
	var envelope struct {
		OK   bool         `json:"ok"`
		Data searchReport `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode search output: %v\n%s", err, out.String())
	}
	if !envelope.OK || envelope.Data.Results == nil {
		t.Fatalf("expected success envelope and non-null results: %s", out.String())
	}
	return envelope.Data
}

func TestSchemaSearchPaginationAndRoundTrip(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		name          string
		flags         []string
		limit, offset int
		want          []string
		more          bool
	}{
		{"default", nil, 10, 0, []string{"im messages list", "mail user_mailbox.messages get", "mail user_mailbox.messages list"}, false},
		{"first", []string{"--limit", "1"}, 1, 0, []string{"im messages list"}, true},
		{"middle", []string{"--offset", "1", "--limit", "1"}, 1, 1, []string{"mail user_mailbox.messages get"}, true},
		{"last", []string{"--offset", "2", "--limit", "100"}, 100, 2, []string{"mail user_mailbox.messages list"}, false},
		{"past end", []string{"--offset", "3"}, 10, 3, []string{}, false},
		{"max offset", []string{"--offset", strconv.Itoa(math.MaxInt)}, 10, math.MaxInt, []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, out, errOut, _ := cmdutil.TestFactory(t, nil)
			f.APICatalog = schemaSurfaceCatalog()
			cmd := NewCmdSchema(f, nil)
			query := "  MeThOd\t "
			cmd.SetArgs(append([]string{"--search", query}, tc.flags...))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			report := decodeSearchReport(t, out)
			if report.Query != query || report.Total != 3 || report.Limit != tc.limit || report.Offset != tc.offset || report.HasMore != tc.more {
				t.Fatalf("pagination or query changed: %+v", report)
			}
			names := make([]string, 0, len(report.Results))
			for _, result := range report.Results {
				names = append(names, result.Name)
				out.Reset()
				full := NewCmdSchema(f, nil)
				full.SetArgs([]string{result.SchemaPath})
				if err := full.Execute(); err != nil {
					t.Fatal(err)
				}
				var schema struct {
					Name        string          `json:"name"`
					InputSchema json.RawMessage `json:"inputSchema"`
				}
				if err := json.Unmarshal(out.Bytes(), &schema); err != nil {
					t.Fatal(err)
				}
				if schema.Name != result.Name || len(schema.InputSchema) == 0 {
					t.Fatalf("schema path did not round trip: %s", out.String())
				}
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("names = %v, want %v", names, tc.want)
			}
			if errOut.Len() != 0 {
				t.Fatalf("unexpected stderr: %s", errOut.String())
			}
		})
	}
}

func TestSchemaSearchPathScopes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"mail", "--search", "method"},
		{"--search=method", "mail.user_mailbox.messages"},
		{"mail", "user_mailbox.messages", "--search", "method"},
	} {
		f, out, _, _ := cmdutil.TestFactory(t, nil)
		f.APICatalog = schemaSurfaceCatalog()
		cmd := NewCmdSchema(f, nil)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		report := decodeSearchReport(t, out)
		if report.Total != 2 {
			t.Fatalf("scope %v returned %+v", args, report)
		}
		for _, result := range report.Results {
			if result.CommandPath[0] != "mail" {
				t.Fatalf("scope expanded: %+v", result)
			}
		}
	}
}

func TestSchemaSearchValidation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		args  []string
		param string
	}{
		{[]string{"--search", ""}, "--search"},
		{[]string{"--search", " \t\n"}, "--search"},
		{[]string{"--search", "a", "--limit", "0"}, "--limit"},
		{[]string{"--search", "a", "--limit", "101"}, "--limit"},
		{[]string{"--search", "a", "--offset", "-1"}, "--offset"},
		{[]string{"--limit", "10"}, "--limit"},
		{[]string{"--offset", "0"}, "--offset"},
	} {
		f, out, _, _ := cmdutil.TestFactory(t, nil)
		cmd := NewCmdSchema(f, nil)
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		cmd.SetArgs(tc.args)
		err := cmd.Execute()
		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != tc.param {
			t.Fatalf("%v: expected invalid_argument with param %s, got %#v", tc.args, tc.param, err)
		}
		if out.Len() != 0 {
			t.Fatalf("invalid query wrote output: %s", out.String())
		}
	}
}

func TestSchemaSearchStrictIdentityBeforePagination(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{{Name: "sample", Resources: map[string]meta.Resource{
		"items": {Methods: map[string]meta.Method{
			"bot":  {AccessTokens: []meta.Token{meta.TokenTenant}},
			"user": {AccessTokens: []meta.Token{meta.TokenUser}},
		}},
	}}})
	for _, tc := range []struct {
		identities uint8
		want, as   string
	}{
		{2, "sample items bot", "user"}, {1, "sample items user", "bot"},
	} {
		f, out, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "test-secret", SupportedIdentities: tc.identities})
		f.APICatalog = catalog
		cmd := NewCmdSchema(f, nil)
		cmd.SetArgs([]string{"--search", "items", "--limit", "1", "--as", tc.as})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		report := decodeSearchReport(t, out)
		if report.Total != 1 || report.HasMore || len(report.Results) != 1 || report.Results[0].Name != tc.want {
			t.Fatalf("identity filter applied after pagination or overridden by --as: %+v", report)
		}
	}
}

func TestSchemaSearchVisibilityBeforePagination(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	visible := func(path []string) bool { return strings.Join(path, "/") != "mail/user_mailbox.messages/list" }
	for _, tc := range []struct {
		query      string
		visibility CommandVisibility
		total      int
	}{
		{"method", visible, 2}, {"concealable", visible, 0}, {"method", func([]string) bool { return false }, 0}, {"absent", nil, 0},
	} {
		f, out, _, _ := cmdutil.TestFactory(t, nil)
		f.APICatalog = schemaSurfaceCatalog()
		cmd := NewCmdSchemaWithVisibility(f, tc.visibility, nil)
		cmd.SetArgs([]string{"--search", tc.query, "--offset", "1", "--limit", "1"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		report := decodeSearchReport(t, out)
		if report.Total != tc.total || report.HasMore || len(report.Results) != max(0, tc.total-1) || strings.Contains(out.String(), "concealable") && len(report.Results) != 0 {
			t.Fatalf("concealed result affected counts/page: %+v", report)
		}
	}
	f, out, _, _ := cmdutil.TestFactory(t, nil)
	f.APICatalog = schemaSurfaceCatalog()
	cmd := NewCmdSchemaWithVisibility(f, visible, nil)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"mail.user_mailbox.messages.list", "--search", "method"})
	var validationErr *errs.ValidationError
	if err := cmd.Execute(); !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("concealed scope error = %v", err)
	}
	if strings.Contains(validationErr.Hint, "list") || !strings.Contains(validationErr.Hint, "get") || out.Len() != 0 {
		t.Fatalf("concealed scope leaked into hints/output: %v / %s", validationErr, out.String())
	}
}

type searchFailureLoader struct{ failure error }

func (searchFailureLoader) Names() []string { return []string{"broken", "im"} }
func (l searchFailureLoader) Load(name string) (meta.Service, error) {
	if name == "broken" {
		return meta.Service{}, l.failure
	}
	svc, _ := schemaSurfaceCatalog().Service(name)
	return svc, nil
}

func TestSchemaSearchLazyCatalogFailures(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cause := errors.New("corrupt test metadata")
	failure := errs.NewInternalError(errs.SubtypeUnknown, "cannot load catalog").WithCause(cause)
	for _, args := range [][]string{{"--search", "method", "--limit", "1"}, {"broken", "--search", "method"}, {"im", "--search", "method"}} {
		f, out, _, _ := cmdutil.TestFactory(t, nil)
		f.APICatalog = apicatalog.NewLazy(apicatalog.SourceEmbedded, searchFailureLoader{failure})
		cmd := NewCmdSchema(f, nil)
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		cmd.SetArgs(args)
		err := cmd.Execute()
		if args[0] == "im" {
			if err != nil {
				t.Fatalf("scoped search loaded unrelated shard: %v", err)
			}
			if report := decodeSearchReport(t, out); report.Total != 1 {
				t.Fatalf("scoped results: %+v", report)
			}
			continue
		}
		if err != failure || !errors.Is(err, cause) || out.Len() != 0 {
			t.Fatalf("partial success or lost error cause: %v / %s", err, out.String())
		}
	}
}

type failingSearchWriter struct{}

func (failingSearchWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestSchemaSearchOutputFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.APICatalog = schemaSurfaceCatalog()
	f.IOStreams.Out = failingSearchWriter{}
	cmd := NewCmdSchema(f, nil)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"--search", "method"})
	err := cmd.Execute()
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output failure lost typed error/cause: %v", err)
	}
}
