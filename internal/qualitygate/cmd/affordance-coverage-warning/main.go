// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// coverage-warning reports native API methods that lack affordance Markdown.
// It is a manual inventory tool: missing documents are warnings, never a
// failing exit status.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/registry"
)

func main() {
	affordance.SetSource(os.DirFS("affordance"))
	reportCoverage(
		os.Stdout,
		registry.EmbeddedCatalog().WalkMethods(nil),
		func(service, methodID string) bool {
			_, ok := affordance.For(service, methodID)
			return ok
		},
	)
}

func reportCoverage(out io.Writer, methods []apicatalog.MethodRef, documented func(service, methodID string) bool) {
	if len(methods) == 0 {
		fmt.Fprintln(out, "WARNING: no embedded API metadata; run make fetch_meta before checking coverage.")
		return
	}
	var missing []string
	for _, method := range methods {
		service, methodID := method.ServiceName(), method.Method.ID
		if strings.HasPrefix(methodID, "+") {
			continue
		}
		if !documented(service, methodID) {
			missing = append(missing, service+"/"+methodID)
		}
	}
	sort.Strings(missing)
	if len(missing) == 0 {
		fmt.Fprintln(out, "Affordance coverage is complete.")
		return
	}
	fmt.Fprintf(out, "WARNING: %d native command(s) have no affordance document:\n", len(missing))
	for _, key := range missing {
		fmt.Fprintln(out, key)
	}
}
