// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcredential "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/surface"
	"github.com/spf13/cobra"
)

type offlineMeetingManagementTestCredentialProvider struct {
	name string
}

func (p *offlineMeetingManagementTestCredentialProvider) Name() string { return p.name }
func (p *offlineMeetingManagementTestCredentialProvider) ResolveAccount(context.Context) (*extcredential.Account, error) {
	return nil, nil
}
func (p *offlineMeetingManagementTestCredentialProvider) ResolveToken(
	context.Context,
	extcredential.TokenSpec,
) (*extcredential.Token, error) {
	return nil, nil
}

func cleanOfflineMeetingManagementEligibilityProbes() offlineMeetingManagementEligibilityProbes {
	return offlineMeetingManagementEligibilityProbes{
		registeredPluginCount: func() int { return 0 },
		credentialProviderSnapshot: func() ([]offlineMeetingManagementCredentialProvider, bool) {
			return []offlineMeetingManagementCredentialProvider{standardOfflineMeetingManagementCredentialProvider}, true
		},
		stat:          func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist },
		getenv:        func(string) string { return "" },
		baseConfigDir: func() string { return "/isolated/lark-cli" },
	}
}

func useCleanOfflineMeetingManagementEligibility(t *testing.T) {
	t.Helper()
	original := offlineMeetingManagementEligibilityProbesForInvocation
	offlineMeetingManagementEligibilityProbesForInvocation = cleanOfflineMeetingManagementEligibilityProbes
	t.Cleanup(func() { offlineMeetingManagementEligibilityProbesForInvocation = original })
}

func TestDescribeOfflineMeetingManagementCredentialProvidersFailsClosed(t *testing.T) {
	var nilProvider *offlineMeetingManagementTestCredentialProvider
	tests := []struct {
		name      string
		providers []extcredential.Provider
	}{
		{name: "nil provider", providers: []extcredential.Provider{nil}},
		{name: "typed nil provider", providers: []extcredential.Provider{nilProvider}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if snapshot, determinate := describeOfflineMeetingManagementCredentialProviders(test.providers); determinate || snapshot != nil {
				t.Fatalf("provider snapshot = %#v, determinate = %v; want nil, false", snapshot, determinate)
			}
		})
	}
}

func TestOfflineMeetingManagementEligibilityFailsClosed(t *testing.T) {
	tests := []struct {
		name         string
		inv          cmdutil.InvocationContext
		presentation restrictionPresentationConfig
		configure    func(*offlineMeetingManagementEligibilityProbes)
		want         bool
	}{
		{name: "clean default process", want: true},
		{name: "distribution concealment requires the full surface", presentation: restrictionPresentationConfig{enabled: true}},
		{name: "environment profile", inv: cmdutil.InvocationContext{ProfileSource: core.ProfileFromEnvironment}},
		{name: "explicit profile", inv: cmdutil.InvocationContext{Profile: "uat", ProfileSource: core.ProfileFromFlag}},
		{name: "registered platform plugin", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.registeredPluginCount = func() int { return 1 }
		}},
		{name: "missing standard credential provider", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) { return nil, true }
		}},
		{name: "nil credential provider makes snapshot indeterminate", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) { return nil, false }
		}},
		{name: "credential provider snapshot panics", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) { panic("provider name unavailable") }
		}},
		{name: "sidecar credential provider", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) {
				return []offlineMeetingManagementCredentialProvider{{
					name:        "sidecar",
					packagePath: "github.com/larksuite/cli/extension/credential/sidecar",
					typeName:    "Provider",
				}}, true
			}
		}},
		{name: "custom provider cannot spoof env by name", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) {
				return []offlineMeetingManagementCredentialProvider{{
					name:        "env",
					packagePath: "example.com/wrapper/credential",
					typeName:    "Provider",
				}}, true
			}
		}},
		{name: "additional credential provider", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) {
				return []offlineMeetingManagementCredentialProvider{
					standardOfflineMeetingManagementCredentialProvider,
					{name: "custom", packagePath: "example.com/wrapper/credential", typeName: "Provider"},
				}, true
			}
		}},
		{name: "missing credential provider snapshot probe", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = nil
		}},
		{name: "policy file exists", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.stat = func(path string) (fs.FileInfo, error) {
				if strings.HasSuffix(path, "policy.yml") {
					return nil, nil
				}
				return nil, fs.ErrNotExist
			}
		}},
		{name: "policy stat is indeterminate", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.stat = func(string) (fs.FileInfo, error) { return nil, errors.New("permission denied") }
		}},
		{name: "config file exists and may carry strict mode", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.stat = func(path string) (fs.FileInfo, error) {
				if strings.HasSuffix(path, "config.json") {
					return nil, nil
				}
				return nil, fs.ErrNotExist
			}
		}},
		{name: "strict mode environment signal", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.getenv = func(key string) string {
				if key == "LARKSUITE_CLI_STRICT_MODE" {
					return "bot"
				}
				return ""
			}
		}},
		{name: "credential environment signal", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.getenv = func(key string) string {
				if key == "LARKSUITE_CLI_USER_ACCESS_TOKEN" {
					return "present"
				}
				return ""
			}
		}},
		{name: "missing stat probe", configure: func(p *offlineMeetingManagementEligibilityProbes) { p.stat = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probes := cleanOfflineMeetingManagementEligibilityProbes()
			if test.configure != nil {
				test.configure(&probes)
			}
			if got := canRunOfflineMeetingManagementPreflight(test.inv, test.presentation, probes); got != test.want {
				t.Fatalf("eligibility = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOfflineMeetingManagementIneligibleRootInvocationFallsBack(t *testing.T) {
	originalProbes := offlineMeetingManagementEligibilityProbesForInvocation
	t.Cleanup(func() { offlineMeetingManagementEligibilityProbesForInvocation = originalProbes })

	tests := []struct {
		name         string
		inv          cmdutil.InvocationContext
		args         []string
		presentation restrictionPresentationConfig
		configure    func(*offlineMeetingManagementEligibilityProbes)
	}{
		{name: "distribution concealment", presentation: restrictionPresentationConfig{enabled: true}},
		{name: "deny plugin", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.registeredPluginCount = func() int { return 1 }
		}},
		{name: "conceal plugin", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.registeredPluginCount = func() int { return 1 }
		}},
		{name: "missing standard credential provider", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) { return nil, true }
		}},
		{name: "nil credential provider makes snapshot indeterminate", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) { return nil, false }
		}},
		{name: "credential provider snapshot panic", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) { panic("provider name unavailable") }
		}},
		{name: "sidecar credential provider", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) {
				return []offlineMeetingManagementCredentialProvider{{
					name:        "sidecar",
					packagePath: "github.com/larksuite/cli/extension/credential/sidecar",
					typeName:    "Provider",
				}}, true
			}
		}},
		{name: "custom provider cannot spoof env by name", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) {
				return []offlineMeetingManagementCredentialProvider{{
					name:        "env",
					packagePath: "example.com/wrapper/credential",
					typeName:    "Provider",
				}}, true
			}
		}},
		{name: "additional custom credential provider", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.credentialProviderSnapshot = func() ([]offlineMeetingManagementCredentialProvider, bool) {
				return []offlineMeetingManagementCredentialProvider{
					standardOfflineMeetingManagementCredentialProvider,
					{name: "wrapper", packagePath: "example.com/wrapper/credential", typeName: "Provider"},
				}, true
			}
		}},
		{name: "yaml policy", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.stat = func(path string) (fs.FileInfo, error) {
				if strings.HasSuffix(path, "policy.yml") {
					return nil, nil
				}
				return nil, fs.ErrNotExist
			}
		}},
		{name: "environment profile availability", inv: cmdutil.InvocationContext{Profile: "session", ProfileSource: core.ProfileFromEnvironment}},
		{
			name: "explicit profile availability",
			inv:  cmdutil.InvocationContext{Profile: "uat", ProfileSource: core.ProfileFromFlag},
			args: []string{"--profile", "uat", "vc", "+meeting-end", "--meeting-id", "1"},
		},
		{name: "persisted strict mode may exist", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.stat = func(path string) (fs.FileInfo, error) {
				if strings.HasSuffix(path, "config.json") {
					return nil, nil
				}
				return nil, fs.ErrNotExist
			}
		}},
		{name: "policy stat uncertainty", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.stat = func(string) (fs.FileInfo, error) { return nil, errors.New("permission denied") }
		}},
		{name: "strict mode environment signal", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.getenv = func(key string) string {
				if key == "LARKSUITE_CLI_STRICT_MODE" {
					return "user"
				}
				return ""
			}
		}},
		{name: "credential environment signal", configure: func(p *offlineMeetingManagementEligibilityProbes) {
			p.getenv = func(key string) string {
				if key == "LARKSUITE_CLI_APP_ID" {
					return "configured"
				}
				return ""
			}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probes := cleanOfflineMeetingManagementEligibilityProbes()
			if test.configure != nil {
				test.configure(&probes)
			}
			offlineMeetingManagementEligibilityProbesForInvocation = func() offlineMeetingManagementEligibilityProbes { return probes }

			var stdout, stderr bytes.Buffer
			streams := cmdutil.NewIOStreams(strings.NewReader(""), &stdout, &stderr)
			args := test.args
			if args == nil {
				args = []string{"vc", "+meeting-end", "--meeting-id", "1"}
			}
			terminal, code := runOfflineMeetingManagementPreflight(
				context.Background(),
				test.inv,
				test.presentation,
				streams,
				args,
			)
			if terminal || code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("ineligible root invocation must fall back untouched; terminal=%v code=%d stdout=%q stderr=%q", terminal, code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestOfflineMeetingManagementInvocationRecognition(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "end", args: []string{"vc", "+meeting-end", "--meeting-id", "1"}, want: true},
		{name: "kickout with root profile", args: []string{"--profile", "uat", "vc", "+meeting-participant-kickout", "--meeting-id", "1", "--participant", "2=1"}, want: true},
		{name: "profile equals after group", args: []string{"vc", "--profile=uat", "+meeting-end", "--meeting-id", "1"}, want: true},
		{name: "help uses full presentation", args: []string{"vc", "+meeting-end", "--help"}, want: false},
		{name: "version uses full presentation", args: []string{"--version"}, want: false},
		{name: "other vc command", args: []string{"vc", "+detail", "--meeting-id", "1"}, want: false},
		{name: "target text as another command value", args: []string{"im", "messages", "list", "--query", "vc", "+meeting-end"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isOfflineMeetingManagementInvocation(test.args); got != test.want {
				t.Fatalf("isOfflineMeetingManagementInvocation(%q) = %v, want %v", test.args, got, test.want)
			}
		})
	}
}

func TestOfflineMeetingManagementTerminalPathsSkipFullStartup(t *testing.T) {
	useCleanOfflineMeetingManagementEligibility(t)

	originalSingleApp := singleAppModeForInvocation
	originalBrand := resolveStartupBrandForInvocation
	originalBuild := buildFullInvocation
	originalNotice := output.PendingNotice
	t.Cleanup(func() {
		singleAppModeForInvocation = originalSingleApp
		resolveStartupBrandForInvocation = originalBrand
		buildFullInvocation = originalBuild
		output.PendingNotice = originalNotice
	})

	singleAppModeForInvocation = func() bool {
		t.Fatal("offline terminal path read config through isSingleAppMode")
		return false
	}
	resolveStartupBrandForInvocation = func(string) core.LarkBrand {
		t.Fatal("offline terminal path read config through ResolveStartupBrand")
		return ""
	}
	buildFullInvocation = func(context.Context, cmdutil.InvocationContext, *buildConfig) (*buildRuntime, *cobra.Command, *hook.Registry) {
		t.Fatal("offline terminal path entered the full config/credential/plugin build")
		return nil, nil, nil
	}
	output.PendingNotice = func() map[string]interface{} {
		t.Fatal("offline terminal path evaluated a process-global notice provider")
		return nil
	}

	tests := []struct {
		name        string
		args        []string
		wantCode    int
		wantOutput  string
		wantType    errs.Category
		wantSubtype errs.Subtype
		wantParam   string
	}{
		{name: "end missing required", args: []string{"vc", "+meeting-end"}, wantCode: 2, wantOutput: "required flag", wantType: errs.CategoryValidation, wantSubtype: errs.SubtypeInvalidArgument},
		{name: "end business validation", args: []string{"vc", "+meeting-end", "--meeting-id", "0"}, wantCode: 2, wantOutput: "positive base-10 int64", wantType: errs.CategoryValidation, wantSubtype: errs.SubtypeInvalidArgument, wantParam: "--meeting-id"},
		{name: "end dry run", args: []string{"vc", "+meeting-end", "--meeting-id", "1", "--dry-run", "--as", "user"}, wantCode: 0, wantOutput: "/open-apis/vc/v1/meetings/1/end"},
		{name: "end bot dry run", args: []string{"vc", "+meeting-end", "--meeting-id", "1", "--dry-run", "--as", "bot"}, wantCode: 0, wantOutput: "/open-apis/vc/v1/bots/end"},
		{name: "end dry run rejects unresolved omitted identity", args: []string{"vc", "+meeting-end", "--meeting-id", "1", "--dry-run"}, wantCode: 2, wantOutput: "requires explicit --as user or --as bot", wantType: errs.CategoryValidation, wantSubtype: errs.SubtypeInvalidArgument, wantParam: "--as"},
		{name: "end dry run rejects unresolved auto identity", args: []string{"vc", "+meeting-end", "--meeting-id", "1", "--dry-run", "--as", "auto"}, wantCode: 2, wantOutput: "requires explicit --as user or --as bot", wantType: errs.CategoryValidation, wantSubtype: errs.SubtypeInvalidArgument, wantParam: "--as"},
		{name: "end confirmation", args: []string{"vc", "+meeting-end", "--meeting-id", "1"}, wantCode: 10, wantOutput: "confirmation", wantType: errs.CategoryConfirmation, wantSubtype: errs.SubtypeConfirmationRequired},
		{name: "kickout invalid identity", args: []string{"vc", "+meeting-participant-kickout", "--meeting-id", "1", "--participant", "2=1", "--as", "bot"}, wantCode: 2, wantOutput: "only supports: user", wantType: errs.CategoryValidation, wantSubtype: errs.SubtypeInvalidArgument, wantParam: "--as"},
		{name: "kickout validation", args: []string{"vc", "+meeting-participant-kickout", "--meeting-id", "1", "--participant", " 2=1"}, wantCode: 2, wantOutput: "surrounding whitespace", wantType: errs.CategoryValidation, wantSubtype: errs.SubtypeInvalidArgument, wantParam: "--participant"},
		{name: "kickout dry run", args: []string{"vc", "+meeting-participant-kickout", "--meeting-id", "1", "--participant", "2=1", "--dry-run", "--as", "user"}, wantCode: 0, wantOutput: "kickout_users"},
		{name: "kickout confirmation", args: []string{"vc", "+meeting-participant-kickout", "--meeting-id", "1", "--participant", "2=1"}, wantCode: 10, wantOutput: "confirmation", wantType: errs.CategoryConfirmation, wantSubtype: errs.SubtypeConfirmationRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := executeWithCapturedOS(t, nil, test.args...)
			if code != test.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%s stderr=%s", code, test.wantCode, stdout, stderr)
			}
			if combined := stdout + stderr; !strings.Contains(combined, test.wantOutput) {
				t.Fatalf("output does not contain %q; stdout=%s stderr=%s", test.wantOutput, stdout, stderr)
			}
			if test.wantType == "" {
				return
			}
			var envelope struct {
				OK    bool `json:"ok"`
				Error struct {
					Type    errs.Category `json:"type"`
					Subtype errs.Subtype  `json:"subtype"`
					Param   *string       `json:"param"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
				t.Fatalf("stderr is not a typed JSON envelope: %v; stderr=%s", err, stderr)
			}
			if envelope.OK {
				t.Errorf("envelope ok = true, want false")
			}
			if envelope.Error.Type != test.wantType || envelope.Error.Subtype != test.wantSubtype {
				t.Errorf("typed error = %s/%s, want %s/%s; stderr=%s", envelope.Error.Type, envelope.Error.Subtype, test.wantType, test.wantSubtype, stderr)
			}
			if test.wantParam == "" {
				if envelope.Error.Param != nil {
					t.Errorf("error.param = %q, want field omitted", *envelope.Error.Param)
				}
			} else if envelope.Error.Param == nil || *envelope.Error.Param != test.wantParam {
				got := "<omitted>"
				if envelope.Error.Param != nil {
					got = *envelope.Error.Param
				}
				t.Errorf("error.param = %q, want %q", got, test.wantParam)
			}
		})
	}
}

func TestOfflineMeetingManagementYesContinuesToFullStartup(t *testing.T) {
	useCleanOfflineMeetingManagementEligibility(t)

	var stdout, stderr bytes.Buffer
	streams := cmdutil.NewIOStreams(strings.NewReader(""), &stdout, &stderr)
	terminal, code := runOfflineMeetingManagementPreflight(
		context.Background(),
		cmdutil.InvocationContext{},
		restrictionPresentationConfig{},
		streams,
		[]string{"vc", "+meeting-end", "--meeting-id", "1", "--yes"},
	)
	if terminal || code != 0 {
		t.Fatalf("offline --yes result = terminal:%v code:%d, want continuation; stdout=%s stderr=%s", terminal, code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("offline --yes preflight emitted output before full startup; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestExecuteWithConcealmentFallsBackToFullStartup(t *testing.T) {
	useCleanOfflineMeetingManagementEligibility(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	originalSingleApp := singleAppModeForInvocation
	originalBrand := resolveStartupBrandForInvocation
	originalBuild := buildFullInvocation
	originalNotice := output.PendingNotice
	t.Cleanup(func() {
		singleAppModeForInvocation = originalSingleApp
		resolveStartupBrandForInvocation = originalBrand
		buildFullInvocation = originalBuild
		output.PendingNotice = originalNotice
	})

	singleAppModeForInvocation = func() bool { return false }
	resolveStartupBrandForInvocation = func(string) core.LarkBrand { return core.BrandLark }
	fullStartupCalled := false
	config := &core.CliConfig{Brand: core.BrandLark}
	buildFullInvocation = func(_ context.Context, inv cmdutil.InvocationContext, cfg *buildConfig) (*buildRuntime, *cobra.Command, *hook.Registry) {
		fullStartupCalled = true
		if !cfg.presentation.enabled {
			t.Fatal("concealment option was not preserved for the full build")
		}
		root := &cobra.Command{
			Use:                "lark-cli",
			DisableFlagParsing: true,
			RunE:               func(*cobra.Command, []string) error { return nil },
		}
		root.SetIn(cfg.streams.In)
		root.SetOut(cfg.streams.Out)
		root.SetErr(cfg.streams.ErrOut)
		factory, _, _, _ := cmdutil.TestFactory(t, config)
		factory.IOStreams = cfg.streams
		factory.Invocation = inv
		return &buildRuntime{
			Factory: factory,
			surface: surface.NewPlan(map[surface.CommandID]surface.CommandState{
				surface.CommandUpdate: surface.CommandConcealed,
			}),
		}, root, nil
	}

	code, stdout, stderr := executeWithCapturedOS(
		t,
		[]BuildOption{ConcealRestrictedCommands()},
		"vc", "+meeting-end", "--meeting-id", "1",
	)
	if code != 0 || !fullStartupCalled {
		t.Fatalf("concealed invocation code=%d fullStartupCalled=%v; stdout=%s stderr=%s", code, fullStartupCalled, stdout, stderr)
	}
}
