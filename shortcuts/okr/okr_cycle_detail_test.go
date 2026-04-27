// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// --- helpers ---

func cycleDetailTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:     "test-okr-detail-" + suffix,
		AppSecret: "secret-okr-detail-" + suffix,
		Brand:     core.BrandFeishu,
	}
}

func runCycleDetailShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRCycleDetail.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func decodeEnvelope(t *testing.T, stdout *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode output: %v\nraw=%s", err, stdout.String())
	}
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("missing data in output envelope: %#v", envelope)
	}
	return data
}

// --- Validate tests ---

func TestCycleDetailValidate_MissingCycleID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail"})
	if err == nil {
		t.Fatal("expected error for missing --cycle-id")
	}
	// cobra catches required flag before our Validate runs
	if !strings.Contains(err.Error(), "cycle-id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleDetailValidate_InvalidCycleID_NonNumeric(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "abc"})
	if err == nil {
		t.Fatal("expected error for non-numeric --cycle-id")
	}
	if !strings.Contains(err.Error(), "--cycle-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleDetailValidate_InvalidCycleID_Zero(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "0"})
	if err == nil {
		t.Fatal("expected error for zero --cycle-id")
	}
	if !strings.Contains(err.Error(), "--cycle-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleDetailValidate_InvalidCycleID_Negative(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "-1"})
	if err == nil {
		t.Fatal("expected error for negative --cycle-id")
	}
	if !strings.Contains(err.Error(), "--cycle-id must be a positive int64") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleDetailValidate_ValidCycleID(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	// Need to register stubs because Validate passes and Execute runs
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- DryRun tests ---

func TestCycleDetailDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	err := runCycleDetailShortcut(t, f, stdout, []string{
		"+cycle-detail",
		"--cycle-id", "456",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "456") {
		t.Fatalf("dry-run output should contain cycle-id 456, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/cycles/456/objectives") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
}

// --- Execute tests ---

func TestCycleDetailExecute_NoObjectives(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/100/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "100"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	if data["cycle_id"] != "100" {
		t.Fatalf("cycle_id = %v, want 100", data["cycle_id"])
	}
	objs, _ := data["objectives"].([]interface{})
	if len(objs) != 0 {
		t.Fatalf("objectives = %v, want empty", objs)
	}
}

func TestCycleDetailExecute_WithObjectivesAndKeyResults(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleDetailTestConfig(t))

	// Stub for objectives
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/200/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":       "obj-1",
						"owner":    map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"cycle_id": "200",
						"score":    0.8,
						"weight":   1.0,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run": map[string]interface{}{
													"text": "Improve team productivity",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// Stub for key results of obj-1
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/obj-1/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "kr-1",
						"objective_id": "obj-1",
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"score":        0.9,
						"weight":       0.5,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run": map[string]interface{}{
													"text": "Reduce response time by 50%",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "200"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeEnvelope(t, stdout)
	if data["cycle_id"] != "200" {
		t.Fatalf("cycle_id = %v, want 200", data["cycle_id"])
	}
	objs, _ := data["objectives"].([]interface{})
	if len(objs) != 1 {
		t.Fatalf("objectives count = %d, want 1", len(objs))
	}
	obj, _ := objs[0].(map[string]interface{})
	if obj["id"] != "obj-1" {
		t.Fatalf("objective id = %v, want obj-1", obj["id"])
	}
	krs, _ := obj["key_results"].([]interface{})
	if len(krs) != 1 {
		t.Fatalf("key results count = %d, want 1", len(krs))
	}
	kr, _ := krs[0].(map[string]interface{})
	if kr["id"] != "kr-1" {
		t.Fatalf("key result id = %v, want kr-1", kr["id"])
	}
}

func TestCycleDetailExecute_Pagination(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleDetailTestConfig(t))

	// First page of objectives
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/300/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":       "obj-p1",
						"owner":    map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"cycle_id": "300",
						"score":    0.5,
						"weight":   1.0,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run":     map[string]interface{}{"text": "Page1 obj"},
											},
										},
									},
								},
							},
						},
					},
				},
				"has_more":   true,
				"page_token": "next_page_token",
			},
		},
	})

	// Second page of objectives (no more)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/300/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":       "obj-p2",
						"owner":    map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"cycle_id": "300",
						"score":    0.6,
						"weight":   1.0,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run":     map[string]interface{}{"text": "Page2 obj"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// Key results for obj-p1: first page with has_more=true
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/obj-p1/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "kr-p1-1",
						"objective_id": "obj-p1",
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"score":        0.7,
						"weight":       0.5,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run":     map[string]interface{}{"text": "KR page 1 for obj-p1"},
											},
										},
									},
								},
							},
						},
					},
				},
				"has_more":   true,
				"page_token": "kr-p1-next",
			},
		},
	})
	// Key results for obj-p1: second page with has_more=false
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/obj-p1/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "kr-p1-2",
						"objective_id": "obj-p1",
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"score":        0.8,
						"weight":       0.5,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run":     map[string]interface{}{"text": "KR page 2 for obj-p1"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// Key results for obj-p2: first page with has_more=true
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/obj-p2/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "kr-p2-1",
						"objective_id": "obj-p2",
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"score":        0.6,
						"weight":       0.4,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run":     map[string]interface{}{"text": "KR page 1 for obj-p2"},
											},
										},
									},
								},
							},
						},
					},
				},
				"has_more":   true,
				"page_token": "kr-p2-next",
			},
		},
	})
	// Key results for obj-p2: second page with has_more=false
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/obj-p2/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "kr-p2-2",
						"objective_id": "obj-p2",
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"score":        0.9,
						"weight":       0.6,
						"content": map[string]interface{}{
							"blocks": []interface{}{
								map[string]interface{}{
									"block_type": 1,
									"paragraph": map[string]interface{}{
										"elements": []interface{}{
											map[string]interface{}{
												"element_type": 1,
												"text_run":     map[string]interface{}{"text": "KR page 2 for obj-p2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "300"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeEnvelope(t, stdout)
	objs, _ := data["objectives"].([]interface{})
	if len(objs) != 2 {
		t.Fatalf("objectives count = %d, want 2", len(objs))
	}

	// Verify key_results are aggregated across pages for each objective
	for i, objRaw := range objs {
		obj, _ := objRaw.(map[string]interface{})
		objID, _ := obj["id"].(string)
		krs, _ := obj["key_results"].([]interface{})
		if len(krs) != 2 {
			t.Fatalf("objective[%d] %s: key_results count = %d, want 2", i, objID, len(krs))
		}
		// Verify KR IDs are distinct (from different pages)
		krIDs := make(map[string]bool)
		for _, krRaw := range krs {
			kr, _ := krRaw.(map[string]interface{})
			krID, _ := kr["id"].(string)
			krIDs[krID] = true
		}
		if len(krIDs) != 2 {
			t.Fatalf("objective %s: expected 2 distinct KR IDs, got %v", objID, krIDs)
		}
	}
}

func TestCycleDetailExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleDetailTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/400/objectives",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runCycleDetailShortcut(t, f, stdout, []string{"+cycle-detail", "--cycle-id", "400"})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
