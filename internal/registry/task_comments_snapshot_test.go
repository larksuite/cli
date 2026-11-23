// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"testing"

	"github.com/larksuite/cli/internal/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCommentsSnapshot(t *testing.T) {
	snapshot, err := OpenSnapshot()
	require.NoError(t, err)
	catalog := snapshot.Catalog()
	task, ok := catalog.Service("task")
	require.NoError(t, catalog.Err())
	require.True(t, ok)
	assert.Equal(t, "/open-apis/task/v2", task.ServicePath)
	comments, ok := task.Resources["comments"]
	require.True(t, ok, "Task comment APIs must be in the committed catalog")
	require.Len(t, comments.Methods, 5)

	tests := []struct {
		name       string
		path       string
		httpMethod string
		risk       string
		scopes     []string
		tokens     []meta.Token
	}{
		{"create", "comments", "POST", "write", []string{"task:comment:writeonly", "task:comment:write"}, []meta.Token{"user", "tenant", "agent"}},
		{"delete", "comments/{comment_id}", "DELETE", "high-risk-write", []string{"task:comment:delete", "task:comment:write"}, []meta.Token{"tenant", "user"}},
		{"get", "comments/{comment_id}", "GET", "read", []string{"task:comment:read", "task:comment:write"}, []meta.Token{"tenant", "user"}},
		{"list", "comments", "GET", "read", []string{"task:comment:read", "task:comment:write"}, []meta.Token{"user", "tenant", "agent"}},
		{"patch", "comments/{comment_id}", "PATCH", "write", []string{"task:comment:writeonly", "task:comment:write"}, []meta.Token{"tenant", "user"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := catalog.Resolve([]string{"task", "comments", tt.name})
			require.NoError(t, err)
			require.NotNil(t, target.Method)
			method := target.Method.Method
			assert.Equal(t, "comments."+tt.name, method.ID)
			assert.Equal(t, tt.path, method.Path)
			assert.Equal(t, tt.httpMethod, method.HTTPMethod)
			assert.Equal(t, tt.risk, method.Risk)
			assert.ElementsMatch(t, tt.scopes, method.Scopes)
			assert.ElementsMatch(t, tt.tokens, method.AccessTokens)
			assert.Equal(t, []string{"bot", "user"}, method.Identities())

			if tt.name == "delete" || tt.name == "get" || tt.name == "patch" {
				param, ok := method.Parameters["comment_id"]
				require.True(t, ok)
				assert.Equal(t, "string", param.Type)
				assert.Equal(t, "path", param.Location)
				assert.True(t, param.Required)
			}
			if tt.name != "delete" {
				param, ok := method.Parameters["user_id_type"]
				require.True(t, ok)
				assert.Equal(t, "query", param.Location)
				assert.False(t, param.Required)
				assert.Equal(t, "open_id", param.Default)
			}

			if tt.name == "delete" {
				assert.Empty(t, method.RequestBody)
				assert.Empty(t, method.ResponseBody)
				return
			}
			fields := method.ResponseBody["comment"].Properties
			if tt.name == "list" {
				require.Len(t, method.ResponseBody, 3)
				assert.Equal(t, "array", method.ResponseBody["items"].Type)
				assert.Equal(t, "string", method.ResponseBody["page_token"].Type)
				assert.Equal(t, "boolean", method.ResponseBody["has_more"].Type)
				fields = method.ResponseBody["items"].Properties
			} else {
				require.Len(t, method.ResponseBody, 1)
				assert.Equal(t, "object", method.ResponseBody["comment"].Type)
			}
			require.Len(t, fields, 8)
			for _, name := range []string{"id", "content", "reply_to_comment_id", "created_at", "updated_at", "resource_type", "resource_id"} {
				assert.Equal(t, "string", fields[name].Type, name)
			}
			assert.Equal(t, "object", fields["creator"].Type)
			require.Len(t, fields["creator"].Properties, 4)
			for _, name := range []string{"id", "type", "role", "name"} {
				assert.Equal(t, "string", fields["creator"].Properties[name].Type, "creator."+name)
			}

			switch tt.name {
			case "create":
				require.Len(t, method.RequestBody, 4)
				assert.True(t, method.RequestBody["content"].Required)
				assert.Equal(t, "string", method.RequestBody["content"].Type)
				assert.Equal(t, "string", method.RequestBody["reply_to_comment_id"].Type)
				assert.False(t, method.RequestBody["reply_to_comment_id"].Required)
				assert.Equal(t, "task", method.RequestBody["resource_type"].Default)
				assert.Equal(t, "string", method.RequestBody["resource_id"].Type)
				assert.False(t, method.RequestBody["resource_id"].Required)
			case "list":
				assert.Empty(t, method.RequestBody)
				assert.True(t, method.Parameters["resource_id"].Required)
				assert.Equal(t, "query", method.Parameters["resource_id"].Location)
				assert.Equal(t, "task", method.Parameters["resource_type"].Default)
				assert.Equal(t, "asc", method.Parameters["direction"].Default)
				assert.Equal(t, int64(50), method.Parameters["page_size"].CoercedDefault())
				assert.Equal(t, "1", method.Parameters["page_size"].Min)
				assert.Equal(t, "100", method.Parameters["page_size"].Max)
				assert.Equal(t, "string", method.Parameters["page_token"].Type)
				require.Len(t, method.Parameters["direction"].Options, 2)
				assert.Equal(t, "asc", method.Parameters["direction"].Options[0].Value)
				assert.Equal(t, "desc", method.Parameters["direction"].Options[1].Value)
			case "patch":
				require.Len(t, method.RequestBody, 2)
				assert.True(t, method.RequestBody["comment"].Required)
				assert.Equal(t, "object", method.RequestBody["comment"].Type)
				require.Len(t, method.RequestBody["comment"].Properties, 1)
				assert.Equal(t, "string", method.RequestBody["comment"].Properties["content"].Type)
				assert.True(t, method.RequestBody["update_fields"].Required)
				assert.Equal(t, "array", method.RequestBody["update_fields"].Type)
			case "get":
				assert.Empty(t, method.RequestBody)
			}
		})
	}
}
