// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"io"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
)

func TestWikiNodeLookupProblemPreservesUpstreamError(t *testing.T) {
	for _, code := range []int{131012, 131013, 131014, 131016} {
		err := errs.NewAPIError(errs.SubtypeUnknown, "upstream message").WithCode(code).
			WithRetryable().WithLogID("lookup-log").WithHint("upstream hint").WithCause(io.EOF)
		got := wikiNodeLookupProblem(err)
		require.Same(t, err, got)
		require.ErrorIs(t, got, io.EOF)
		p, ok := errs.ProblemOf(got)
		require.True(t, ok)
		require.Equal(t, code, p.Code)
		require.Equal(t, "lookup-log", p.LogID)
		require.Equal(t, "upstream hint", p.Hint)
		require.False(t, p.Retryable)
	}
	require.Nil(t, wikiNodeLookupProblem(nil))
	require.ErrorIs(t, wikiNodeLookupProblem(io.EOF), io.EOF)
}

// Exercise the mounted commands so failures must stop before the write, and
// successful lookups must use resolved tokens with the requested mutation type.
func TestWikiMutationNodeLookup(t *testing.T) {
	for _, command := range []struct {
		name         string
		shortcut     common.Shortcut
		args         []string
		token        string
		method       string
		path         string
		body         map[string]interface{}
		sourceLookup bool
		spaceParam   string
	}{
		{"create infer space", WikiNodeCreate, []string{"+node-create", "--parent-node-token", "wik_input"}, "wik_input", "POST", "/spaces/space_123/nodes", map[string]interface{}{"parent_node_token": "wik_input"}, false, ""},
		{"create assert space", WikiNodeCreate, []string{"+node-create", "--parent-node-token", "wik_input", "--space-id", "space_123"}, "wik_input", "POST", "/spaces/space_123/nodes", map[string]interface{}{"parent_node_token": "wik_input"}, false, "--space-id"},
		{"move source", WikiMove, []string{"+move", "--node-token", "wik_input", "--target-space-id", "space_dst"}, "wik_input", "POST", "/spaces/space_123/nodes/wik_input/move", map[string]interface{}{"target_space_id": "space_dst"}, false, ""},
		{"move parent", WikiMove, []string{"+move", "--node-token", "wik_source", "--source-space-id", "space_src", "--target-parent-token", "wik_input"}, "wik_input", "POST", "/spaces/space_src/nodes/wik_source/move", map[string]interface{}{"target_parent_token": "wik_input"}, true, ""},
		{"delete wiki", WikiNodeDelete, []string{"+node-delete", "--node-token", "wik_input", "--obj-type", "wiki", "--include-children", "--yes"}, "wik_input", "DELETE", "/spaces/space_123/nodes/wik_input", map[string]interface{}{"obj_type": "wiki", "include_children": true}, false, ""},
		{"delete document", WikiNodeDelete, []string{"+node-delete", "--node-token", "obj_input", "--obj-type", "docx", "--include-children=false", "--yes"}, "obj_input", "DELETE", "/spaces/space_123/nodes/obj_input", map[string]interface{}{"obj_type": "docx", "include_children": false}, false, ""},
		{"create document parent", WikiNodeCreate, []string{"+node-create", "--parent-node-token", "obj_input"}, "obj_input", "POST", "/spaces/space_123/nodes", map[string]interface{}{"parent_node_token": "wik_input"}, false, ""},
		{"create document parent explicit space", WikiNodeCreate, []string{"+node-create", "--parent-node-token", "obj_input", "--space-id", "space_123"}, "obj_input", "POST", "/spaces/space_123/nodes", map[string]interface{}{"parent_node_token": "wik_input"}, false, "--space-id"},
		{"move document source", WikiMove, []string{"+move", "--node-token", "obj_input", "--target-space-id", "space_dst"}, "obj_input", "POST", "/spaces/space_123/nodes/wik_input/move", map[string]interface{}{"target_space_id": "space_dst"}, false, ""},
		{"move document source explicit space", WikiMove, []string{"+move", "--node-token", "obj_input", "--source-space-id", "space_123", "--target-space-id", "space_dst"}, "obj_input", "POST", "/spaces/space_123/nodes/wik_input/move", map[string]interface{}{"target_space_id": "space_dst"}, false, "--source-space-id"},
		{"move parent document", WikiMove, []string{"+move", "--node-token", "wik_source", "--source-space-id", "space_src", "--target-parent-token", "obj_input"}, "obj_input", "POST", "/spaces/space_src/nodes/wik_source/move", map[string]interface{}{"target_parent_token": "wik_input"}, true, ""},
		{"move parent explicit target space", WikiMove, []string{"+move", "--node-token", "wik_source", "--source-space-id", "space_src", "--target-parent-token", "obj_input", "--target-space-id", "space_123"}, "obj_input", "POST", "/spaces/space_src/nodes/wik_source/move", map[string]interface{}{"target_parent_token": "wik_input", "target_space_id": "space_123"}, true, "--target-space-id"},
		{"delete document as wiki", WikiNodeDelete, []string{"+node-delete", "--node-token", "obj_input", "--obj-type", "wiki", "--yes"}, "obj_input", "DELETE", "/spaces/space_123/nodes/wik_input", map[string]interface{}{"obj_type": "wiki"}, false, ""},
		{"delete document as wiki explicit space", WikiNodeDelete, []string{"+node-delete", "--node-token", "obj_input", "--obj-type", "wiki", "--space-id", "space_123", "--yes"}, "obj_input", "DELETE", "/spaces/space_123/nodes/wik_input", map[string]interface{}{"obj_type": "wiki"}, false, "--space-id"},
		{"delete wiki as document", WikiNodeDelete, []string{"+node-delete", "--node-token", "wik_input", "--obj-type", "docx", "--space-id", "space_123", "--yes"}, "wik_input", "DELETE", "/spaces/space_123/nodes/obj_input", map[string]interface{}{"obj_type": "docx"}, false, "--space-id"},
	} {
		t.Run(command.name, func(t *testing.T) {
			for _, scenario := range []struct {
				name string
				code int
			}{
				{"origin", 0}, {"shortcut", 0}, {"missing identity", 0}, {"wrong type", 0}, {"wrong space", 0},
				{"invalid parameters", 131002}, {"not found", 131005}, {"forbidden", 131006}, {"deleted", 131012},
				{"invalid token", 131013}, {"outside Wiki", 131014}, {"short token", 131016},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())
					node := map[string]interface{}{"space_id": "space_123", "node_token": "wik_input", "obj_token": "obj_input", "obj_type": "docx", "node_type": "origin"}
					response := map[string]interface{}{"code": 0, "data": map[string]interface{}{"node": node}}
					wantCategory, wantSubtype := errs.CategoryValidation, errs.SubtypeInvalidArgument
					success := scenario.name == "origin" || scenario.name == "shortcut"
					if scenario.name == "shortcut" && command.body["obj_type"] == "docx" {
						success = false
					}
					switch scenario.name {
					case "shortcut":
						node["node_type"], node["origin_node_token"] = "shortcut", "wik_origin"
					case "wrong type":
						if command.body["obj_type"] != "docx" {
							t.Skip("only document-token deletion specifies the underlying type")
						}
						node["obj_type"] = "sheet"
					case "missing identity":
						delete(node, "node_token")
						delete(node, "obj_token")
						wantCategory, wantSubtype = errs.CategoryInternal, errs.SubtypeInvalidResponse
					case "wrong space":
						if command.spaceParam == "" {
							t.Skip("this case does not assert the lookup's space")
						}
						node["space_id"] = "space_other"
					}
					code := scenario.code
					if code != 0 {
						response = map[string]interface{}{"code": code, "msg": "lookup failed"}
						wantCategory, wantSubtype = errs.CategoryAPI, errs.SubtypeUnknown
						switch code {
						case 131002, 131013, 131016:
							wantSubtype = errs.SubtypeInvalidParameters
						case 131005, 131012:
							wantSubtype = errs.SubtypeNotFound
						case 131014:
							wantSubtype = errs.SubtypeFailedPrecondition
						case 131006:
							wantCategory, wantSubtype = errs.CategoryAuthorization, errs.SubtypePermissionDenied
						}
					}
					if command.sourceLookup {
						reg.Register(&httpmock.Stub{Method: "GET", URL: "node_by_token?token=wik_source", Body: map[string]interface{}{
							"code": 0, "data": map[string]interface{}{"node": map[string]interface{}{"space_id": "space_src", "node_token": "wik_source"}},
						}})
					}
					reg.Register(&httpmock.Stub{
						Method: "GET", URL: "/open-apis/wiki/v2/spaces/node_by_token", Status: http.StatusOK,
						Body: response, Headers: http.Header{"X-Tt-Logid": []string{"lookup-log"}},
						OnMatch: func(req *http.Request) {
							require.Equal(t, "/open-apis/wiki/v2/spaces/node_by_token", req.URL.Path)
							require.Equal(t, "token="+command.token, req.URL.RawQuery)
						},
					})
					write := &httpmock.Stub{
						Method: command.method, URL: "/open-apis/wiki/v2" + command.path, Optional: !success,
						OnMatch: func(req *http.Request) {
							require.True(t, success, "failed lookup must not reach a mutation")
							require.Equal(t, "/open-apis/wiki/v2"+command.path, req.URL.Path)
						},
						Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"node": node}},
					}
					reg.Register(write)
					args := append(append([]string{}, command.args...), "--as", "user")
					err := mountAndRunWiki(t, command.shortcut, args, factory, stdout)
					if success {
						require.NoError(t, err)
						require.Empty(t, stderr.String())
						require.Len(t, write.CapturedBodies, 1)
						body := decodeWikiCapturedJSONBody(t, write)
						for key, value := range command.body {
							require.Equal(t, value, body[key], "mutation field %s", key)
						}
					} else {
						require.Error(t, err)
						p, ok := errs.ProblemOf(err)
						require.True(t, ok)
						require.Equal(t, wantCategory, p.Category)
						require.Equal(t, wantSubtype, p.Subtype)
						require.Equal(t, code, p.Code)
						require.False(t, p.Retryable)
						if code != 0 {
							require.Equal(t, "lookup-log", p.LogID)
						}
						if scenario.name == "wrong space" {
							var validation *errs.ValidationError
							require.ErrorAs(t, err, &validation)
							require.Equal(t, command.spaceParam, validation.Param)
						}
						if scenario.name == "wrong type" || (scenario.name == "shortcut" && !success) {
							var validation *errs.ValidationError
							require.ErrorAs(t, err, &validation)
							require.Equal(t, "--obj-type", validation.Param)
						}
						require.Empty(t, stdout.String())
						require.Empty(t, write.CapturedBodies)
					}
				})
			}
		})
	}
}
