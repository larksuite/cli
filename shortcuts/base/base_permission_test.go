// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
)

func TestPermissionShortcutRequests(t *testing.T) {
	cases := []struct {
		shortcut                       common.Shortcut
		args                           []string
		method, suffix, body, response string
	}{
		{BaseRoleMemberList, []string{"--role-id", "rol_test", "--page-size", "2", "--page-token", "2"}, "GET", "/roles/rol_test/members", "", `{"items":[],"has_more":false}`},
		{BaseRoleMemberAdd, []string{"--role-id", "rol_test", "--user-id", "ou_test", "--user-id", "ou_test", "--department-id", "od_test", "--chat-id", "oc_test"}, "POST", "/roles/rol_test/members/batch_create", `{"members":[{"member_id":"ou_test","member_type":"openid"},{"member_id":"od_test","member_type":"opendepartmentid"},{"member_id":"oc_test","member_type":"openchat"}]}`, `{"failed_members":[]}`},
		{BaseRoleMemberRemove, []string{"--role-id", "rol_test", "--chat-id", "oc_test"}, "POST", "/roles/rol_test/members/batch_delete", `{"members":[{"member_id":"oc_test","member_type":"openchat"}]}`, `{"failed_members":[]}`},
		{BaseAdvpermShareGet, nil, "GET", "/advperm/share", "", `{"enable_share":false}`},
		{BaseAdvpermShareUpdate, []string{"--enable-share=false"}, "PUT", "/advperm/share", `{"enable_share":false}`, `{}`},
		{BaseAdvpermShareUpdate, []string{"--enable-share=true"}, "PUT", "/advperm/share", `{"enable_share":true}`, `{}`},
		{BaseAdvpermApply, []string{"--message", " Please review "}, "POST", "/advperm/apply", `{"message":" Please review "}`, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.shortcut.Command+tc.body, func(t *testing.T) {
			for _, identity := range []string{"user", "bot"} {
				if identity == "bot" && tc.shortcut.Command == "+advperm-apply" {
					continue // The separate validation case asserts bot rejection.
				}
				for _, dry := range []bool{true, false} {
					t.Run(identity+"/"+map[bool]string{true: "dry", false: "execute"}[dry], func(t *testing.T) {
						t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
						factory, stdout, reg := newExecuteFactory(t)
						path := "/open-apis/base/v3/bases/base_test" + tc.suffix
						encoded, err := json.Marshal(tc.response)
						require.NoError(t, err)
						stub := &httpmock.Stub{Method: tc.method, URL: path, RawBody: []byte(`{"code":0,"data":{"data":` + string(encoded) + `}}`), OnMatch: func(req *http.Request) {
							require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
							require.NotEmpty(t, req.Header.Get("X-Cli-Shortcut"))
							require.NotEmpty(t, req.Header.Get("X-Cli-Execution-Id"))
							if tc.method == "GET" && strings.Contains(path, "members") {
								require.Equal(t, "2", req.URL.Query().Get("page_size"))
								require.Equal(t, "2", req.URL.Query().Get("page_token"))
							}
						}}
						args := append([]string{tc.shortcut.Command, "--base-token", "base_test", "--as", identity}, tc.args...)
						if dry {
							args = append(args, "--dry-run")
						} else {
							reg.Register(stub)
							if tc.shortcut.Risk == "high-risk-write" {
								args = append(args, "--yes")
							}
						}
						require.NoError(t, runShortcutWithAuthTypes(t, tc.shortcut, nil, args, factory, stdout))
						if dry {
							var result struct {
								Data struct {
									API []struct {
										Method, URL string
										Params      map[string]interface{}
										Body        json.RawMessage
									} `json:"api"`
								} `json:"data"`
							}
							require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
							require.Len(t, result.Data.API, 1)
							call := result.Data.API[0]
							require.Equal(t, tc.method, call.Method)
							require.Equal(t, path, call.URL)
							if tc.body != "" {
								require.JSONEq(t, tc.body, string(call.Body))
							}
							if strings.Contains(path, "members") && tc.method == "GET" {
								require.EqualValues(t, 2, call.Params["page_size"])
								require.Equal(t, "2", call.Params["page_token"])
							}
						} else {
							if tc.body != "" {
								require.JSONEq(t, tc.body, string(stub.CapturedBody))
							}
							var result struct {
								OK   bool
								Data json.RawMessage
							}
							require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
							require.True(t, result.OK)
							require.JSONEq(t, tc.response, string(result.Data))
						}
					})
				}
			}
		})
	}
}

// TestPermissionShortcutHighRiskGate pins that removing role members, which
// revokes their access, refuses to run without --yes and sends nothing.
func TestPermissionShortcutHighRiskGate(t *testing.T) {
	for _, tc := range []struct {
		s    common.Shortcut
		args []string
	}{
		{BaseRoleMemberRemove, []string{"--role-id", "rol_test", "--chat-id", "oc_test"}},
	} {
		t.Run(tc.s.Command, func(t *testing.T) {
			require.Equal(t, "high-risk-write", tc.s.Risk)
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _ := newExecuteFactory(t)
			args := append([]string{tc.s.Command, "--base-token", "base_test", "--as", "user"}, tc.args...)
			err := runShortcutWithAuthTypes(t, tc.s, nil, args, f, out)
			require.Error(t, err)
			require.True(t, errs.IsConfirmationRequired(err), "missing --yes must be a confirmation refusal, got %v", err)
			require.Empty(t, out.String())
		})
	}
	require.Equal(t, "write", BaseRoleMemberAdd.Risk, "adding members stays a plain write")
	require.Equal(t, "write", BaseAdvpermShareUpdate.Risk, "matches +form-share-update and +dashboard-share-update")
}

func TestPermissionShortcutValidation(t *testing.T) {
	for _, tc := range []struct {
		s    common.Shortcut
		args []string
	}{
		{BaseRoleMemberList, []string{"--role-id", "rol_test", "--page-size", "0"}},
		{BaseRoleMemberList, []string{"--role-id", "rol_test", "--page-size", "101"}},
		{BaseRoleMemberAdd, []string{"--role-id", "rol_test"}},
		{BaseRoleMemberAdd, []string{"--role-id", "rol_test", "--user-id", " "}},
		{BaseRoleMemberRemove, []string{"--role-id", "../x", "--chat-id", "oc_test"}},
		{BaseAdvpermShareUpdate, nil},
		{BaseAdvpermApply, []string{"--base-token", "https://example.test/base/x"}},
	} {
		t.Run(tc.s.Command+strings.Join(tc.args, "_"), func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, _ := newExecuteFactory(t)
			args := append([]string{tc.s.Command, "--base-token", "base_test", "--dry-run", "--as", "user"}, tc.args...)
			err := runShortcutWithAuthTypes(t, tc.s, nil, args, f, out)
			require.True(t, errs.IsValidation(err), "%v", err)
			require.Empty(t, out.String())
		})
	}
	t.Run("apply rejects bot", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
		f, out, _ := newExecuteFactory(t)
		require.Error(t, runShortcutWithAuthTypes(t, BaseAdvpermApply, nil, []string{"+advperm-apply", "--base-token", "base_test", "--as", "bot", "--dry-run"}, f, out))
	})
}

func TestPermissionShortcutFailureEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		partial   bool
		code      int
	}{
		{"outer denied", `{"code":3,"msg":"denied"}`, false, 3},
		{"inner denied", `{"code":0,"data":{"code":3,"message":"denied"}}`, false, 3},
		{"partial outer", `{"code":1,"data":{"data":"{\"failed_members\":[{\"member_id\":\"oc_test\",\"member_type\":\"openchat\",\"result_code\":3}]}"}}`, true, 0},
		{"partial inner", `{"code":0,"data":{"code":1,"data":{"failed_members":[{"member_id":"oc_test","result_code":3}]}}}`, true, 0},
		{"invalid payload", `{"code":0,"data":{"data":"invalid"}}`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/base_test/roles/rol_test/members/batch_create", RawBody: []byte(tc.raw)})
			err := runShortcutWithAuthTypes(t, BaseRoleMemberAdd, nil, []string{"+role-member-add", "--base-token", "base_test", "--role-id", "rol_test", "--chat-id", "oc_test", "--as", "user"}, f, out)
			require.Error(t, err)
			if tc.partial {
				var partial *output.PartialFailureError
				require.ErrorAs(t, err, &partial)
				var result struct {
					OK   bool
					Data json.RawMessage
				}
				require.NoError(t, json.Unmarshal(out.Bytes(), &result))
				require.False(t, result.OK)
				var payload struct {
					FailedMembers []struct {
						MemberID   string `json:"member_id"`
						ResultCode int    `json:"result_code"`
					} `json:"failed_members"`
				}
				require.NoError(t, json.Unmarshal(result.Data, &payload))
				require.Len(t, payload.FailedMembers, 1)
				require.Equal(t, "oc_test", payload.FailedMembers[0].MemberID)
				require.Equal(t, 3, payload.FailedMembers[0].ResultCode)
			} else {
				p, ok := errs.ProblemOf(err)
				require.True(t, ok)
				if tc.code != 0 {
					require.Equal(t, tc.code, p.Code)
				}
				require.Empty(t, out.String())
			}
		})
	}
}

// TestPermissionShortcutPartialFailureKeepsMemberResults ensures failed member
// details stay machine-readable and the command exits unsuccessfully.
func TestPermissionShortcutPartialFailureKeepsMemberResults(t *testing.T) {
	const failed = `{"failed_members":[{"member_id":"oc_test","member_type":"openchat","result_code":3}]}`
	for _, tc := range []struct {
		name, raw string
	}{
		{"outer refusal", `{"code":1,"msg":"caller is not a Base admin","log_id":"perm-test-log","data":` + failed + `}`},
		// The backend's shape whenever any member fails: the PARTIAL_FAILURE
		// error sits beside the string-encoded data, not inside it.
		{"backend partial failure", `{"code":1,"msg":"some role members could not be synchronized","log_id":"perm-test-log","data":{"error":{"type":"PARTIAL_FAILURE","message":"some role members could not be synchronized","hint":"inspect failed_members and list current role members before retrying"},"data":"{\"failed_members\":[{\"member_id\":\"oc_test\",\"member_type\":\"openchat\",\"result_code\":3}]}"}}`},
		{"inner refusal", `{"code":0,"data":{"code":1,"message":"caller is not a Base admin","error":{"log_id":"perm-test-log"},"data":` + failed + `}}`},
		{"plain partial success", `{"code":0,"data":` + failed + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/base/v3/bases/base_test/roles/rol_test/members/batch_create", RawBody: []byte(tc.raw)})
			err := runShortcutWithAuthTypes(t, BaseRoleMemberAdd, nil, []string{"+role-member-add", "--base-token", "base_test", "--role-id", "rol_test", "--chat-id", "oc_test", "--as", "user"}, f, out)
			var partial *output.PartialFailureError
			require.ErrorAs(t, err, &partial)
			var result struct {
				OK   bool            `json:"ok"`
				Data json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.False(t, result.OK)
			require.Contains(t, string(result.Data), `"failed_members"`)
		})
	}
}

// TestPermissionShortcutBareAcknowledgement pins that a {code:0} envelope with
// no data, the normal reply to a write with nothing to return, decodes to an
// empty object rather than leaking the envelope's code/msg as the payload.
func TestPermissionShortcutBareAcknowledgement(t *testing.T) {
	for name, raw := range map[string]string{
		"outer":  `{"code":0,"msg":"success"}`,
		"inner":  `{"code":0,"data":{"code":0,"msg":"success"}}`,
		"string": `{"code":0,"data":{"data":"{\"code\":0,\"msg\":\"success\"}"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, out, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "PUT", URL: "/open-apis/base/v3/bases/base_test/advperm/share", RawBody: []byte(raw)})
			err := runShortcutWithAuthTypes(t, BaseAdvpermShareUpdate, nil, []string{"+advperm-share-update", "--base-token", "base_test", "--enable-share=true", "--as", "user"}, f, out)
			require.NoError(t, err)
			var result struct {
				OK   bool                       `json:"ok"`
				Data map[string]json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &result))
			require.True(t, result.OK)
			require.NotContains(t, result.Data, "code", "transport envelope must not surface as business data")
			require.NotContains(t, result.Data, "msg")
			require.Empty(t, result.Data)
		})
	}
}

func TestPermissionShortcutErrorDetails(t *testing.T) {
	const detail = `{"code":3,"message":"denied","error":{"hint":"ask a Base admin","log_id":"permission-test-log"}}`
	encoded, err := json.Marshal(detail)
	require.NoError(t, err)
	for name, raw := range map[string]string{
		"outer":        detail,
		"inner object": `{"code":0,"data":` + detail + `}`,
		"inner string": `{"code":0,"data":{"data":` + string(encoded) + `}}`,
		"nested data":  `{"code":0,"data":{"code":3,"message":"denied","data":{"error":{"hint":"ask a Base admin","log_id":"permission-test-log"}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			factory, out, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/base_test/advperm/share", RawBody: []byte(raw)})
			err := runShortcutWithAuthTypes(t, BaseAdvpermShareGet, nil, []string{"+advperm-share-get", "--base-token", "base_test", "--as", "user"}, factory, out)
			problem, ok := errs.ProblemOf(err)
			require.True(t, ok)
			require.Equal(t, 3, problem.Code)
			require.Equal(t, "ask a Base admin", problem.Hint)
			require.Equal(t, "permission-test-log", problem.LogID)
			require.Empty(t, out.String())
		})
	}
}

func TestPermissionShortcutMemberLimit(t *testing.T) {
	for _, shortcut := range []common.Shortcut{BaseRoleMemberAdd, BaseRoleMemberRemove} {
		for _, count := range []int{100, 101} {
			t.Run(fmt.Sprintf("%s/%d", shortcut.Command, count), func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				factory, out, _ := newExecuteFactory(t)
				args := []string{shortcut.Command, "--base-token", "base_test", "--role-id", "rol_test", "--dry-run", "--as", "user"}
				for i := 0; i < count; i++ {
					args = append(args, "--user-id", fmt.Sprintf("ou_test_%d", i))
				}
				err := runShortcutWithAuthTypes(t, shortcut, nil, args, factory, out)
				if count == 101 {
					require.True(t, errs.IsValidation(err))
					require.Empty(t, out.String())
					return
				}
				require.NoError(t, err)
				var result struct {
					Data struct {
						API []struct {
							Body permissionMemberBatch
						}
					}
				}
				require.NoError(t, json.Unmarshal(out.Bytes(), &result))
				require.Len(t, result.Data.API, 1)
				require.Len(t, result.Data.API[0].Body.Members, 100)
			})
		}
	}
}

func TestPermissionShortcutTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
	}{
		{"HTTP failure with batch payload", http.StatusForbidden, nil},
		{"transport error", 0, io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			factory, out, reg := newExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "POST", URL: "/open-apis/base/v3/bases/base_test/roles/rol_test/members/batch_create",
				Status: tc.status, Error: tc.err,
				RawBody: []byte(`{"code":1,"data":{"failed_members":[{"member_id":"oc_test","result_code":3}]}}`),
			})
			err := runShortcutWithAuthTypes(t, BaseRoleMemberAdd, nil, []string{"+role-member-add", "--base-token", "base_test", "--role-id", "rol_test", "--chat-id", "oc_test", "--as", "user"}, factory, out)
			_, ok := errs.ProblemOf(err)
			require.True(t, ok, "expected a typed failure, got %v", err)
			require.Empty(t, out.String(), "transport/HTTP failures must not emit a batch success or partial result")
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			}
		})
	}
}
