// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/spf13/cobra"
)

func TestMeetingDomainHelpUsesUnifiedSkill(t *testing.T) {
	affordance.SetSource(os.DirFS("../../affordance"))
	t.Cleanup(func() { affordance.SetSource(nil) })
	skillContent := os.DirFS("../../skills")

	for _, domainName := range []string{"vc", "minutes", "note"} {
		t.Run(domainName, func(t *testing.T) {
			root := &cobra.Command{Use: "lark-cli"}
			domain := &cobra.Command{Use: domainName, Short: domainName}
			cmdmeta.SetSource(domain, cmdmeta.SourceService, false)
			cmdmeta.SetDomain(domain, domainName)
			domain.AddCommand(&cobra.Command{Use: "resource", Run: func(*cobra.Command, []string) {}})
			root.AddCommand(domain)

			if !PrepareDomainHelp(domain, skillContent) {
				t.Fatal("PrepareDomainHelp returned false")
			}
			if !strings.Contains(domain.Long, "lark-cli skills read lark-meeting") {
				t.Fatalf("domain help does not use the unified meeting skill:\n%s", domain.Long)
			}
			legacyPointer := "lark-cli skills read lark-" + domainName
			if strings.Contains(domain.Long, legacyPointer) {
				t.Fatalf("domain help still uses legacy skill pointer %q:\n%s", legacyPointer, domain.Long)
			}
		})
	}
}
