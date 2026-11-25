// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestSchemaValidationThroughRoot(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		exit        int
	}{
		{"documented-example", `{"params":{"page_size":50,"direction":"ASC"}}`, 0},
		{"invalid-fields", `{"params":{"page_size":201,"direction":"PRIVATE_VALUE"}}`, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
			var rootOut, rootErr bytes.Buffer
			root, err := buildRootForArgs(context.Background(), buildInvocationForTest(t),
				[]string{"schema", "drive.files.list", "--validate", tt.input},
				WithoutPlugins(), WithoutStrictMode(), WithIO(strings.NewReader(""), &rootOut, &rootErr))
			if err != nil {
				t.Fatal(err)
			}
			root.SetArgs([]string{"schema", "drive.files.list", "--validate", tt.input})
			exit := 0
			if err := root.Execute(); err != nil {
				exit = handleRootError(f, err, nil)
			}
			if exit != tt.exit || stdout.Len() != 0 || rootErr.Len() != 0 {
				t.Fatalf("exit=%d stdout=%s root stderr=%s", exit, stdout, &rootErr)
			}
			if tt.exit == 0 {
				var envelope struct {
					OK   bool `json:"ok"`
					Data struct {
						SchemaPath string `json:"schema_path"`
						Valid      bool   `json:"valid"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rootOut.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if !envelope.OK || !envelope.Data.Valid || envelope.Data.SchemaPath != "drive.files.list" || stderr.Len() > 0 {
					t.Fatalf("stdout=%s stderr=%s", &rootOut, stderr)
				}
				return
			}
			envelope := parseTypedEnvelope(t, stderr)
			if envelope.OK || envelope.Error.Type != "validation" || envelope.Error.Subtype != "invalid_argument" || envelope.Error.Param != "--validate" || len(envelope.Error.Params) != 2 || rootOut.Len() > 0 {
				t.Fatalf("stdout=%s stderr=%s", &rootOut, stderr)
			}
			if envelope.Error.Params[0].Name != "/params/direction" || envelope.Error.Params[1].Name != "/params/page_size" || strings.Contains(stderr.String(), "PRIVATE_VALUE") {
				t.Fatal(stderr.String())
			}
		})
	}
}
