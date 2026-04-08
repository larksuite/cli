// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Test coverage preview:
//
//	| Workflow | Commands |
//	| --- | --- |
//	| table / field / record / view lifecycle | base +base-create, base +table-create, base +table-list, base +table-get, base +table-update, base +table-delete, base +field-create, base +field-list, base +field-get, base +field-update, base +field-search-options, base +field-delete, base +record-upsert, base +record-list, base +record-get, base +record-history-list, base +record-upload-attachment, base +record-delete, base +view-create, base +view-list, base +view-get, base +view-rename, base +view-set-filter, base +view-get-filter, base +view-set-group, base +view-get-group, base +view-set-sort, base +view-get-sort, base +view-set-timebar, base +view-get-timebar, base +view-set-card, base +view-get-card, base +view-delete, base +data-query |
func TestBase_TableFieldRecordViewWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, "lark-cli-e2e-base-main-"+testSuffix())
	tableID, primaryFieldID, primaryViewID := createTable(t, parentT, ctx, baseToken, "lark-cli-e2e-orders-"+testSuffix(), `[{"name":"Name","type":"text"}]`, `{"name":"Main","type":"grid"}`)
	require.NotEmpty(t, primaryFieldID)
	require.NotEmpty(t, primaryViewID)

	statusFieldID := createField(t, parentT, ctx, baseToken, tableID, `{"name":"Status","type":"select","multiple":false,"options":[{"name":"Open","hue":"Blue"},{"name":"Closed","hue":"Green"}]}`)
	noteFieldID := createField(t, parentT, ctx, baseToken, tableID, `{"name":"Note","type":"text"}`)
	attachmentFieldID := createField(t, parentT, ctx, baseToken, tableID, `{"name":"Files","type":"attachment"}`)
	dueFieldID := createField(t, parentT, ctx, baseToken, tableID, `{"name":"Due","type":"datetime","style":{"format":"yyyy/MM/dd"}}`)
	dueEndFieldID := createField(t, parentT, ctx, baseToken, tableID, `{"name":"Due End","type":"datetime","style":{"format":"yyyy/MM/dd"}}`)

	recordID := createRecord(t, parentT, ctx, baseToken, tableID, `{"Name":"Alice","Status":"Open","Note":"Seed row"}`)
	galleryViewID := createView(t, parentT, ctx, baseToken, tableID, `{"name":"Gallery","type":"gallery"}`)
	calendarViewID := createView(t, parentT, ctx, baseToken, tableID, `{"name":"Calendar","type":"calendar"}`)
	deleteViewID := createView(t, parentT, ctx, baseToken, tableID, `{"name":"DeleteMe","type":"grid"}`)

	t.Run("table list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+table-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot table list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(table_id==\""+tableID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("table get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+table-get", "--base-token", baseToken, "--table-id", tableID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot table get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, tableID, gjson.Get(result.Stdout, "data.table.id").String())
	})

	t.Run("table update", func(t *testing.T) {
		newName := "lark-cli-e2e-orders-renamed-" + testSuffix()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+table-update", "--base-token", baseToken, "--table-id", tableID, "--name", newName},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot table update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, newName, gjson.Get(result.Stdout, "data.table.name").String())
	})

	t.Run("field list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+field-list", "--base-token", baseToken, "--table-id", tableID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot field list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(field_id==\""+statusFieldID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("field get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+field-get", "--base-token", baseToken, "--table-id", tableID, "--field-id", statusFieldID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot field get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, statusFieldID, gjson.Get(result.Stdout, "data.field.id").String())
	})

	t.Run("field update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+field-update", "--base-token", baseToken, "--table-id", tableID, "--field-id", noteFieldID, "--json", `{"name":"Note Updated","type":"text"}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot field update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Note Updated", gjson.Get(result.Stdout, "data.field.name").String())
	})

	t.Run("field search options", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+field-search-options", "--base-token", baseToken, "--table-id", tableID, "--field-id", statusFieldID, "--keyword", "Op", "--limit", "10"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot field option search capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.GreaterOrEqual(t, len(gjson.Get(result.Stdout, "data.options").Array()), 1, "stdout:\n%s", result.Stdout)
	})

	t.Run("record list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-list", "--base-token", baseToken, "--table-id", tableID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot record list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.record_id_list.#(==\""+recordID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("record get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-get", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot record get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "Alice", gjson.Get(result.Stdout, "data.record.Name").String())
		assert.True(t, gjson.Get(result.Stdout, "data.record.Status.0").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("record update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-upsert", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID, "--json", `{"Status":"Closed","Note Updated":"Done"}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot record update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.updated").Bool(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "Closed", gjson.Get(result.Stdout, "data.record.update.Status.0").String())
	})

	t.Run("record history list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-history-list", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID, "--page-size", "10"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot record history capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#").Int() >= 0, "stdout:\n%s", result.Stdout)
	})

	t.Run("record upload attachment", func(t *testing.T) {
		filePath := writeTempAttachment(t, "hello attachment")
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-upload-attachment", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID, "--field-id", attachmentFieldID, "--file", filePath, "--name", "attachment.txt"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot attachment upload capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, true, gjson.Get(result.Stdout, "data.updated").Bool(), "stdout:\n%s", result.Stdout)
	})

	t.Run("data query", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+data-query", "--base-token", baseToken, "--dsl", `{"datasource":{"type":"table","table":{"tableId":"` + tableID + `"}},"dimensions":[{"field_name":"Status","alias":"dim_status"}],"measures":[{"field_name":"Status","aggregation":"count","alias":"status_count"}],"shaper":{"format":"flat"}}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot base data query capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("view list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-list", "--base-token", baseToken, "--table-id", tableID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot view list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(view_id==\""+galleryViewID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("view get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-get", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot view get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, primaryViewID, gjson.Get(result.Stdout, "data.view.id").String())
	})

	t.Run("view rename", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-rename", "--base-token", baseToken, "--table-id", tableID, "--view-id", deleteViewID, "--name", "DeleteSoon"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot view rename capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "DeleteSoon", gjson.Get(result.Stdout, "data.view.name").String())
	})

	t.Run("view set and get filter", func(t *testing.T) {
		setResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-set-filter", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID, "--json", `{"logic":"and","conditions":[["Status","intersects",["Closed"]]]}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if setResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, setResult, "requires bot view filter update capability")
		}
		setResult.AssertExitCode(t, 0)
		setResult.AssertStdoutStatus(t, true)

		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-get-filter", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if getResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, getResult, "requires bot view filter read capability")
		}
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
		assert.Equal(t, "Closed", gjson.Get(getResult.Stdout, "data.filter.conditions.0.2.0").String(), "stdout:\n%s", getResult.Stdout)
	})

	t.Run("view set and get group", func(t *testing.T) {
		setResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-set-group", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID, "--json", `[{"field":"` + statusFieldID + `","desc":false}]`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if setResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, setResult, "requires bot view group update capability")
		}
		setResult.AssertExitCode(t, 0)
		setResult.AssertStdoutStatus(t, true)

		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-get-group", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if getResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, getResult, "requires bot view group read capability")
		}
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
	})

	t.Run("view set and get sort", func(t *testing.T) {
		setResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-set-sort", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID, "--json", `[{"field":"` + statusFieldID + `","desc":true}]`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if setResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, setResult, "requires bot view sort update capability")
		}
		setResult.AssertExitCode(t, 0)
		setResult.AssertStdoutStatus(t, true)

		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-get-sort", "--base-token", baseToken, "--table-id", tableID, "--view-id", primaryViewID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if getResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, getResult, "requires bot view sort read capability")
		}
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
	})

	t.Run("view set and get timebar", func(t *testing.T) {
		setResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-set-timebar", "--base-token", baseToken, "--table-id", tableID, "--view-id", calendarViewID, "--json", `{"start_time":"` + dueFieldID + `","end_time":"` + dueEndFieldID + `","title":"` + primaryFieldID + `"}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if setResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, setResult, "requires bot view timebar update capability")
		}
		setResult.AssertExitCode(t, 0)
		setResult.AssertStdoutStatus(t, true)

		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-get-timebar", "--base-token", baseToken, "--table-id", tableID, "--view-id", calendarViewID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if getResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, getResult, "requires bot view timebar read capability")
		}
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
	})

	t.Run("view set and get card", func(t *testing.T) {
		setResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-set-card", "--base-token", baseToken, "--table-id", tableID, "--view-id", galleryViewID, "--json", `{"cover_field":"` + attachmentFieldID + `"}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if setResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, setResult, "requires bot view card update capability")
		}
		setResult.AssertExitCode(t, 0)
		setResult.AssertStdoutStatus(t, true)

		getResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-get-card", "--base-token", baseToken, "--table-id", tableID, "--view-id", galleryViewID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if getResult.ExitCode != 0 {
			skipIfBaseUnavailable(t, getResult, "requires bot view card read capability")
		}
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
	})

	t.Run("record delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-delete", "--base-token", baseToken, "--table-id", tableID, "--record-id", recordID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot record delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, recordID, gjson.Get(result.Stdout, "data.record_id").String())
	})

	t.Run("view delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+view-delete", "--base-token", baseToken, "--table-id", tableID, "--view-id", deleteViewID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot view delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, deleteViewID, gjson.Get(result.Stdout, "data.view_id").String())
	})

	t.Run("field delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+field-delete", "--base-token", baseToken, "--table-id", tableID, "--field-id", noteFieldID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot field delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, noteFieldID, gjson.Get(result.Stdout, "data.field_id").String())
	})

	t.Run("table delete", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+table-delete", "--base-token", baseToken, "--table-id", tableID, "--yes"},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot table delete capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, tableID, gjson.Get(result.Stdout, "data.table_id").String())
	})
}
