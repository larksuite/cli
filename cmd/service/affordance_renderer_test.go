// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

func TestHelpRendererUsesItsCatalogForIrregularCommandForms(t *testing.T) {
	source := fstest.MapFS{
		"drive.md": {Data: []byte("# drive\n\n## files list\nList files through the injected mapping.\n")},
	}
	service := meta.ServiceFromMap(map[string]interface{}{
		"name": "drive",
		"resources": map[string]interface{}{
			"files": map[string]interface{}{
				"methods": map[string]interface{}{
					"list": map[string]interface{}{"id": "file.list", "httpMethod": "GET"},
				},
			},
		},
	})
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{service})
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List files",
		Annotations: map[string]string{
			schemaPathAnnotation: "drive.files.list",
		},
	}
	cmdmeta.SetAffordanceRef(cmd, "drive", "file.list")

	r := &HelpRenderer{Guidance: affordance.NewResolver(source, catalog)}
	if !r.PrepareMethodHelp(cmd) {
		t.Fatal("PrepareMethodHelp rejected a method command")
	}
	if !strings.Contains(cmd.Long, "List files through the injected mapping.") {
		t.Fatalf("catalog-aware help lost the irregular command-form mapping:\n%s", cmd.Long)
	}

	// The same overlay resolved without the catalog cannot map "files list" to
	// "file.list", so the guidance is absent — the catalog is what makes the
	// heading resolve.
	bare := &cobra.Command{Use: "list", Short: "List files", Annotations: map[string]string{schemaPathAnnotation: "drive.files.list"}}
	cmdmeta.SetAffordanceRef(bare, "drive", "file.list")
	(&HelpRenderer{Guidance: affordance.NewResolver(source, apicatalog.Catalog{})}).PrepareMethodHelp(bare)
	if strings.Contains(bare.Long, "injected mapping") {
		t.Fatalf("guidance resolved without the catalog mapping:\n%s", bare.Long)
	}
}
