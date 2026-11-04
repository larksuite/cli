// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/registry"
)

func TestMailThreadsSchemaParameterConstraints(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	svc, err := snapshot.Load("mail")
	if err != nil {
		t.Fatal(err)
	}
	method := svc.Resources["user_mailbox.threads"].Methods["list"]
	data, err := json.Marshal(buildInputSchema(method))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ParameterConstraints struct{ ExactlyOneOf []string }
		Properties           struct{ Params struct{ Required []string } }
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.ParameterConstraints.ExactlyOneOf, []string{"folder_id", "label_id"}) {
		t.Fatalf("schema lost constraint: %s", data)
	}
	if !reflect.DeepEqual(schema.Properties.Params.Required, []string{"page_size", "user_mailbox_id"}) {
		t.Fatalf("schema changed individual requirements: %s", data)
	}
}
