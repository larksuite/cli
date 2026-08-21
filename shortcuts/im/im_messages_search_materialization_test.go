// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestNewMaterializationLedgerDeduplicatesRequestedIDsAndCountsUnresolvedHits(t *testing.T) {
	ledger := newMaterializationLedger([]interface{}{
		searchHit("om_a"),
		searchHit("om_a"),
		searchHit("om_b"),
		map[string]interface{}{"meta_data": map[string]interface{}{}},
		map[string]interface{}{"meta_data": map[string]interface{}{"message_id": ""}},
		"invalid-hit",
	})

	if got, want := ledger.requestedIDs(), []string{"om_a", "om_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requested IDs = %#v, want %#v", got, want)
	}
	status := ledger.status()
	if status.UnresolvedHitCount != 3 {
		t.Fatalf("unresolved hit count = %d, want 3", status.UnresolvedHitCount)
	}
	if got, want := status.MissingMessageIDs, []string{"om_a", "om_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing IDs = %#v, want %#v", got, want)
	}
}

func TestReconcileMessageMaterializationUsesBatchAllowlistAndDeduplicatesResponses(t *testing.T) {
	ledger := newMaterializationLedger([]interface{}{
		searchHit("om_a"),
		searchHit("om_b"),
	})
	const unexpectedID = "om_secret_unexpected"
	items := reconcileMessageMaterialization(ledger, []string{"om_a", "om_b"}, []interface{}{
		messageDetail("om_a"),
		messageDetail("om_a"),
		messageDetail(unexpectedID),
		map[string]interface{}{"msg_type": "text"},
		messageDetail("om_b"),
	})

	if len(items) != 2 {
		t.Fatalf("resolved items = %#v, want exactly two allowlisted unique items", items)
	}
	status := ledger.status()
	if got, want := status.ResolvedIDs, []string{"om_a", "om_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved IDs = %#v, want %#v", got, want)
	}
	if status.UnexpectedMessageCount != 2 {
		t.Fatalf("unexpected count = %d, want 2", status.UnexpectedMessageCount)
	}
	if len(status.MissingMessageIDs) != 0 {
		t.Fatalf("missing IDs = %#v, want none", status.MissingMessageIDs)
	}

	encoded, err := json.Marshal(struct {
		Items  []interface{}
		Status interface{}
	}{Items: items, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), unexpectedID) {
		t.Fatalf("unknown response ID leaked from reconciliation: %s", encoded)
	}
}

func TestMaterializationLedgerPreservesResolvedItemsAndTypedCause(t *testing.T) {
	ledger := newMaterializationLedger([]interface{}{
		searchHit("om_a"),
		searchHit("om_b"),
		searchHit("om_c"),
	})
	items := reconcileMessageMaterialization(ledger, []string{"om_a", "om_b"}, []interface{}{
		messageDetail("om_a"),
		messageDetail("om_b"),
	})
	cause := errs.NewNetworkError(errs.SubtypeNetworkTransport, "mget unavailable").
		WithCause(errors.New("connection reset"))
	ledger.recordCause(cause)

	status := ledger.status()
	if got, want := status.ResolvedIDs, []string{"om_a", "om_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved IDs = %#v, want %#v", got, want)
	}
	if got, want := status.MissingMessageIDs, []string{"om_c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing IDs = %#v, want %#v", got, want)
	}
	if !errors.Is(status.Cause, cause) {
		t.Fatalf("cause = %v, want typed cause %v", status.Cause, cause)
	}
	if len(items) != 2 {
		t.Fatalf("completed batch items = %#v, want preserved", items)
	}
}

func TestReconcileMessageMaterializationRejectsIDRequestedByAnotherBatch(t *testing.T) {
	ledger := newMaterializationLedger([]interface{}{
		searchHit("om_a"),
		searchHit("om_b"),
	})
	items := reconcileMessageMaterialization(ledger, []string{"om_a"}, []interface{}{
		messageDetail("om_b"),
	})

	if len(items) != 0 {
		t.Fatalf("items = %#v, want cross-batch response discarded", items)
	}
	status := ledger.status()
	if status.UnexpectedMessageCount != 1 {
		t.Fatalf("unexpected count = %d, want 1", status.UnexpectedMessageCount)
	}
	if got, want := status.MissingMessageIDs, []string{"om_a", "om_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missing IDs = %#v, want %#v", got, want)
	}
}

func TestImMessagesSearchMaterializationPartialLedgerAndUnknownResponseIsolation(t *testing.T) {
	const unexpectedID = "om_secret_unexpected"
	runtime := newMessagesSearchRuntime(t,
		map[string]string{"query": "incident", "page-limit": "0"},
		map[string]bool{"page-all": true, "no-reactions": true},
		shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/search"):
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{
							searchHit("om_a"),
							searchHit("om_b"),
							map[string]interface{}{"meta_data": map[string]interface{}{}},
						},
						"has_more": false,
					},
				}), nil
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget"):
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{
							buildMessageDetails([]string{"om_a"})[0],
							buildMessageDetails([]string{"om_a"})[0],
							buildMessageDetails([]string{unexpectedID})[0],
						},
					},
				}), nil
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"items": []interface{}{}},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))
	attachMessagesSearchReadSession(t, runtime)

	if err := ImMessagesSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	envelope, stdout := messagesSearchEnvelope(t, runtime)
	if strings.Contains(stdout, unexpectedID) {
		t.Fatalf("unknown response ID leaked to output: %s", stdout)
	}
	if got, _ := envelope["ok"].(bool); got {
		t.Fatalf("ok = true, want false: %#v", envelope)
	}
	pagination, _ := requireIMReadEnvelopeMeta(t, envelope)
	if got, _ := pagination["complete"].(bool); !got {
		t.Fatalf("meta.pagination.complete = false, want exhausted search pagination: %#v", pagination)
	}
	data := envelope["data"].(map[string]interface{})
	if got, want := data["message_ids"], []interface{}{"om_a", "om_b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("message_ids = %#v, want %#v", got, want)
	}
	if got := data["messages"].([]interface{}); len(got) != 1 {
		t.Fatalf("messages = %#v, want one allowlisted detail", got)
	}
	ledger := data["materialization"].(map[string]interface{})
	assertMaterializationLedger(t, ledger, 2, 1, []interface{}{"om_b"}, 1, 1)
}

func TestImMessagesSearchMaterializationCompleteUsesContractHint(t *testing.T) {
	runtime := newMessagesSearchRuntime(t,
		map[string]string{"query": "incident", "page-limit": "0"},
		map[string]bool{"page-all": true, "no-reactions": true},
		shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/search"):
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items":    []interface{}{searchHit("om_a")},
						"has_more": false,
					},
				}), nil
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget"):
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"items": buildMessageDetails([]string{"om_a"})},
				}), nil
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"items": []interface{}{}},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))
	attachMessagesSearchReadSession(t, runtime)

	if err := ImMessagesSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	envelope, _ := messagesSearchEnvelope(t, runtime)
	if got, _ := envelope["ok"].(bool); !got {
		t.Fatalf("ok = false, want true: %#v", envelope)
	}
	pagination, _ := requireIMReadEnvelopeMeta(t, envelope)
	if got, _ := pagination["complete"].(bool); !got {
		t.Fatalf("meta.pagination.complete = false, want true: %#v", pagination)
	}
	const wantHint = "Results are ready to use. Use message_id/file_key directly; do not call messages-mget."
	if envelope["hint"] != wantHint {
		t.Fatalf("hint = %#v, want %q", envelope["hint"], wantHint)
	}
	data := envelope["data"].(map[string]interface{})
	ledger := data["materialization"].(map[string]interface{})
	if ledger["status"] != "complete" ||
		int(ledger["requested_count"].(float64)) != 1 ||
		int(ledger["resolved_count"].(float64)) != 1 {
		t.Fatalf("materialization ledger = %#v, want complete 1/1", ledger)
	}
}

func TestImMessagesSearchMaterializationPreservesCompletedBatchesOnMGetFailure(t *testing.T) {
	tests := []struct {
		name         string
		failBatch    int
		wantResolved int
		wantMGet     int
	}{
		{name: "first batch", failBatch: 1, wantResolved: 0, wantMGet: 1},
		{name: "later batch", failBatch: 2, wantResolved: messagesSearchMGetBatchSize, wantMGet: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mgetCalls int
			runtime := newMessagesSearchRuntime(t,
				map[string]string{"query": "incident", "page-limit": "0"},
				map[string]bool{"page-all": true, "no-reactions": true},
				shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch {
					case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/search"):
						return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
							"code": 0,
							"data": map[string]interface{}{
								"items":    buildSearchResultItems(1, messagesSearchMGetBatchSize+1),
								"has_more": false,
							},
						}), nil
					case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget"):
						mgetCalls++
						if mgetCalls == tt.failBatch {
							return nil, errors.New("connection reset")
						}
						ids := req.URL.Query()["message_ids"]
						return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
							"code": 0,
							"data": map[string]interface{}{"items": buildMessageDetails(ids)},
						}), nil
					case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/batch_query"):
						return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
							"code": 0,
							"data": map[string]interface{}{"items": []interface{}{}},
						}), nil
					default:
						return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
					}
				}))
			attachMessagesSearchReadSession(t, runtime)

			if err := ImMessagesSearch.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			envelope, _ := messagesSearchEnvelope(t, runtime)
			if got, _ := envelope["ok"].(bool); got {
				t.Fatalf("ok = true, want false: %#v", envelope)
			}
			if mgetCalls != tt.wantMGet {
				t.Fatalf("mget calls = %d, want %d", mgetCalls, tt.wantMGet)
			}
			data := envelope["data"].(map[string]interface{})
			if got := len(data["messages"].([]interface{})); got != tt.wantResolved {
				t.Fatalf("resolved messages = %d, want %d", got, tt.wantResolved)
			}
			ledger := data["materialization"].(map[string]interface{})
			if got := int(ledger["resolved_count"].(float64)); got != tt.wantResolved {
				t.Fatalf("resolved_count = %d, want %d", got, tt.wantResolved)
			}
			if got := len(ledger["missing_message_ids"].([]interface{})); got != messagesSearchMGetBatchSize+1-tt.wantResolved {
				t.Fatalf("missing count = %d, want %d", got, messagesSearchMGetBatchSize+1-tt.wantResolved)
			}
			problem, ok := envelope["error"].(map[string]interface{})
			if !ok || problem["type"] != string(errs.CategoryNetwork) {
				t.Fatalf("error = %#v, want typed network cause", envelope["error"])
			}
		})
	}
}

func attachMessagesSearchReadSession(t *testing.T, runtime *common.RuntimeContext) {
	t.Helper()
	runtime.Format = "json"
	contract, ok := imcontract.Lookup("im +messages-search")
	if !ok {
		t.Fatal("read contract not found")
	}
	session, err := imcontract.NewReadSession(contract, imcontract.ReadOptions{FullRead: true})
	if err != nil {
		t.Fatal(err)
	}
	setRuntimeField(t, runtime, "readSession", session)
}

func messagesSearchEnvelope(t *testing.T, runtime *common.RuntimeContext) (map[string]interface{}, string) {
	t.Helper()
	out := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(out.String()), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	return envelope, out.String()
}

func assertMaterializationLedger(
	t *testing.T,
	ledger map[string]interface{},
	requested, resolved int,
	missing []interface{},
	unresolved, unexpected int,
) {
	t.Helper()
	if ledger["status"] != "partial" ||
		int(ledger["requested_count"].(float64)) != requested ||
		int(ledger["resolved_count"].(float64)) != resolved ||
		!reflect.DeepEqual(ledger["missing_message_ids"], missing) ||
		int(ledger["unresolved_hit_count"].(float64)) != unresolved ||
		int(ledger["unexpected_message_count"].(float64)) != unexpected {
		t.Fatalf("materialization ledger = %#v", ledger)
	}
}

func searchHit(messageID string) map[string]interface{} {
	return map[string]interface{}{
		"meta_data": map[string]interface{}{"message_id": messageID},
	}
}

func messageDetail(messageID string) map[string]interface{} {
	return map[string]interface{}{
		"message_id": messageID,
		"msg_type":   "text",
	}
}
