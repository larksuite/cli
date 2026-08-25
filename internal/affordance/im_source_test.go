// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
)

type imAffordanceExample struct {
	method        string
	command       string
	source        string
	sourceCommand string
	derivation    string
}

var imAffordanceExamples = []imAffordanceExample{
	{method: "+chat-create", command: `lark-cli im +chat-create --name "My Group"`, source: "lark-im/references/lark-im-chat-create.md"},
	{method: "+chat-list", command: "lark-cli im +chat-list", source: "lark-im/references/lark-im-chat-list.md"},
	{method: "+chat-members-list", command: "lark-cli im +chat-members-list --chat-id oc_xxx", source: "lark-im/references/lark-im-chat-members-list.md"},
	{method: "+chat-messages-list", command: "lark-cli im +chat-messages-list --chat-id oc_xxx", source: "lark-im/references/lark-im-chat-messages-list.md"},
	{method: "+chat-search", command: `lark-cli im +chat-search --query "project"`, source: "lark-im/references/lark-im-chat-search.md"},
	{method: "+chat-update", command: `lark-cli im +chat-update --chat-id oc_xxx --name "New Group Name"`, source: "lark-im/references/lark-im-chat-update.md"},
	{method: "+message-read-users", command: "lark-cli im +message-read-users --message-id om_xxx", source: "lark-im/references/lark-im-message-read-status.md"},
	{method: "+messages-mget", command: "lark-cli im +messages-mget --message-ids om_xxx", source: "lark-im/references/lark-im-messages-mget.md"},
	{
		method:        "+messages-read-status",
		command:       "lark-cli im +messages-read-status --as user --message-ids om_xxx,om_yyy",
		source:        "lark-im/references/lark-im-message-read-status.md",
		sourceCommand: "lark-cli im +messages-read-status --message-ids om_xxx,om_yyy --as user --json",
	},
	{method: "+messages-reply", command: `lark-cli im +messages-reply --message-id om_xxx --text "Received"`, source: "lark-im/references/lark-im-messages-reply.md"},
	{method: "+messages-resources-download", command: "lark-cli im +messages-resources-download --message-id om_xxx --file-key img_v3_xxx --type image", source: "lark-im/references/lark-im-messages-resources-download.md"},
	{method: "+messages-search", command: `lark-cli im +messages-search --query "project progress"`, source: "lark-im/references/lark-im-messages-search.md"},
	{method: "+messages-send", command: `lark-cli im +messages-send --chat-id oc_xxx --text "Hello"`, source: "lark-im/references/lark-im-messages-send.md"},
	{method: "+threads-messages-list", command: "lark-cli im +threads-messages-list --thread omt_xxx", source: "lark-im/references/lark-im-threads-messages-list.md"},
	{method: "+flag-create", command: "lark-cli im +flag-create --as user --message-id om_xxx", source: "lark-im/references/lark-im-flag-create.md"},
	{method: "+flag-cancel", command: "lark-cli im +flag-cancel --as user --message-id om_xxx", source: "lark-im/references/lark-im-flag-cancel.md"},
	{method: "+flag-list", command: "lark-cli im +flag-list --as user", source: "lark-im/references/lark-im-flag-list.md"},
	{method: "+feed-shortcut-create", command: "lark-cli im +feed-shortcut-create --as user --chat-id oc_xxx", source: "lark-im/references/lark-im-feed-shortcut-create.md"},
	{method: "+feed-shortcut-remove", command: "lark-cli im +feed-shortcut-remove --as user --chat-id oc_xxx", source: "lark-im/references/lark-im-feed-shortcut-remove.md"},
	{method: "+feed-shortcut-list", command: "lark-cli im +feed-shortcut-list --as user", source: "lark-im/references/lark-im-feed-shortcut-list.md"},
	{method: "+feed-group-list", command: "lark-cli im +feed-group-list --as user", source: "lark-im/references/lark-im-feed-group-list.md"},
	{method: "+feed-group-list-item", command: "lark-cli im +feed-group-list-item --as user --feed-group-id ofg_xxx", source: "lark-im/references/lark-im-feed-group-list-item.md"},
	{method: "+feed-group-query-item", command: "lark-cli im +feed-group-query-item --as user --feed-group-id ofg_xxx --feed-id oc_a,oc_b", source: "lark-im/references/lark-im-feed-group-query-item.md"},
	{
		method:        "chat.members.create",
		command:       `lark-cli im chat.members create --params '{"chat_id":"oc_xxx","member_id_type":"open_id","succeed_type":1}' --data '{"id_list":["ou_aaa","ou_bbb"]}' --as user`,
		source:        "lark-im/references/lark-im-chat-create.md",
		sourceCommand: `lark-cli im chat.members create --params '{"chat_id":"<chat_id from step 2>","member_id_type":"open_id","succeed_type":1}' --data '{"id_list":["ou_aaa","ou_bbb"]}' --as user`,
		derivation:    "materialize-chat-id",
	},
	{method: "feed.groups.create", command: `lark-cli im feed.groups create --as user --data '{"feed_group_creator":{"type":"normal","name":"Releases"}}'`, source: "lark-im/references/lark-im-feed-groups.md"},
	{method: "feed.groups.update", command: `lark-cli im feed.groups update --as user --params '{"feed_group_id":"ofg_xxx"}' --data '{"feed_group_updater":{"name":"测试标签名称","update_fields":[1]}}'`, source: "lark-im/references/lark-im-feed-groups.md"},
	{method: "feed.groups.delete", command: `lark-cli im feed.groups delete --as user --params '{"feed_group_id":"ofg_xxx"}'`, source: "lark-im/references/lark-im-feed-groups.md"},
	{method: "feed.groups.batch_query", command: `lark-cli im feed.groups batch_query --as user --params '{"user_id_type":"open_id"}' --data '{"group_ids":["ofg_xxx","ofg_yyy"]}'`, source: "lark-im/references/lark-im-feed-groups.md"},
	{method: "feed.groups.batch_add_item", command: `lark-cli im feed.groups batch_add_item --as user --params '{"feed_group_id":"ofg_xxx"}' --data '{"items":[{"feed_id":"oc_xxx","feed_type":"chat"},{"feed_id":"oc_yyy","feed_type":"chat"}]}'`, source: "lark-im/references/lark-im-feed-groups.md"},
	{method: "feed.groups.batch_remove_item", command: `lark-cli im feed.groups batch_remove_item --as user --params '{"feed_group_id":"ofg_xxx"}' --data '{"items":[{"feed_id":"oc_xxx","feed_type":"chat"}]}'`, source: "lark-im/references/lark-im-feed-groups.md"},
	{method: "images.create", command: `lark-cli im images create --data '{"image_type":"message"}' --file ./diagram.png`, source: "lark-im/references/lark-im-messages-send.md"},
	{method: "reactions.create", command: `lark-cli im reactions create --params '{"message_id":"om_xxx"}' --data '{"reaction_type":{"emoji_type":"SMILE"}}'`, source: "lark-im/references/lark-im-reactions.md"},
	{method: "reactions.list", command: `lark-cli im reactions list --params '{"message_id":"om_xxx"}'`, source: "lark-im/references/lark-im-reactions.md"},
	{method: "reactions.delete", command: `lark-cli im reactions delete --params '{"message_id":"om_xxx","reaction_id":"ZCaCIjUBVVWSrm5L-3ZTw_xxx"}'`, source: "lark-im/references/lark-im-reactions.md"},
	{
		method:        "reactions.batch_query",
		command:       `lark-cli im reactions batch_query --params '{"user_id_type":"open_id"}' --data '{"queries":[{"message_id":"om_xxx"},{"message_id":"om_yyy"}],"page_size_per_message":10,"reaction_type":"LAUGH"}'`,
		source:        "lark-im/references/lark-im-reactions.md",
		sourceCommand: `lark-cli im reactions batch_query --params '{"user_id_type":"open_id"}' --data '{"queries":[{"message_id":"om_xxx"},{"message_id":"om_yyy","page_token":"<PAGE_TOKEN>"}],"page_size_per_message":10,"reaction_type":"LAUGH"}'`,
		derivation:    "first-page",
	},
}

func TestIMAffordanceExamplesTraceToCurrentSkill(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	if got, ok := DomainSkill("im"); !ok || got != "lark-im" {
		t.Fatalf("DomainSkill(im) = (%q, %v), want (lark-im, true)", got, ok)
	}
	if got, want := len(imAffordanceExamples), 35; got != want {
		t.Fatalf("audited IM example count = %d, want %d", got, want)
	}
	affordanceSource, err := os.ReadFile("../../affordance/im.md")
	if err != nil {
		t.Fatal(err)
	}
	parsedDomain := parseDomainMD(affordanceSource, commandFormResolver("im"))
	if got, want := len(parsedDomain.methods), len(imAffordanceExamples); got != want {
		t.Fatalf("parsed IM affordance entries = %d, audited examples = %d", got, want)
	}
	audited := make(map[string]bool, len(imAffordanceExamples))
	shortcutCount := 0
	for _, example := range imAffordanceExamples {
		audited[example.method] = true
		if strings.HasPrefix(example.method, "+") {
			shortcutCount++
		}
	}
	if shortcutCount != 23 || len(imAffordanceExamples)-shortcutCount != 12 {
		t.Fatalf("audited split = %d shortcuts / %d raw, want 23 / 12", shortcutCount, len(imAffordanceExamples)-shortcutCount)
	}
	for method := range parsedDomain.methods {
		if !audited[method] {
			t.Errorf("IM affordance entry %s bypasses the skill-source audit table", method)
		}
	}

	for _, tt := range imAffordanceExamples {
		t.Run(tt.method, func(t *testing.T) {
			a := parsedIMAffordance(t, tt.method)
			if len(a.Examples) != 1 || a.Examples[0].Command != tt.command {
				t.Fatalf("examples = %#v, want one command %q", a.Examples, tt.command)
			}
			if !containsExact(a.Skills, "lark-im") || !containsExact(a.Skills, tt.source) {
				t.Fatalf("skills = %v, want lark-im and %s", a.Skills, tt.source)
			}

			source, err := os.ReadFile(filepath.Join("../../skills", tt.source))
			if err != nil {
				t.Fatalf("read source skill reference: %v", err)
			}
			sourceCommand := tt.sourceCommand
			if sourceCommand == "" {
				sourceCommand = tt.command
			}
			if !strings.Contains(compactSkillText(string(source)), compactSkillText(sourceCommand)) {
				t.Fatalf("example source %s does not contain audited command %q", tt.source, sourceCommand)
			}
			if tt.derivation != "" {
				materialized := tt.sourceCommand
				switch tt.derivation {
				case "materialize-chat-id":
					materialized = strings.ReplaceAll(materialized, "<chat_id from step 2>", "oc_xxx")
				case "first-page":
					materialized = strings.Replace(materialized, `,"page_token":"<PAGE_TOKEN>"`, "", 1)
				default:
					t.Fatalf("unknown audited derivation %q", tt.derivation)
				}
				if materialized != tt.command {
					t.Fatalf("affordance command is not the audited placeholder materialization:\n got: %s\nwant: %s", tt.command, materialized)
				}
			}
		})
	}
}

func TestIMAffordanceDoesNotDuplicateRuntimeRecovery(t *testing.T) {
	source, err := os.ReadFile("../../affordance/im.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Permissions and recovery", "auth login", "missing_scopes", "console_url"} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("IM affordance duplicates runtime recovery %q", forbidden)
		}
	}
}

func TestIMAffordancePreservesOutboundAndDeleteIntentBoundaries(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	for method, requiredItems := range map[string][]string{
		"+messages-send":  {"recipient", "content", "identity"},
		"+messages-reply": {"target message", "content", "identity"},
	} {
		prerequisites := parsedIMAffordance(t, method).Prerequisites
		for _, required := range requiredItems {
			if !containsItem(prerequisites, required) {
				t.Errorf("%s prerequisites must require confirmed %s: %v", method, required, prerequisites)
			}
		}
	}
	deleteGroup := parsedIMAffordance(t, "feed.groups.delete")
	if !containsItem(deleteGroup.Prerequisites, "exact feed_group_id") || !containsItem(deleteGroup.Prerequisites, "deletion intent") {
		t.Fatalf("feed.groups.delete must preserve the explicit target/intent boundary: %v", deleteGroup.Prerequisites)
	}
}

func TestIMImageUploadExamplesPreserveIdentityChoice(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	for _, path := range []string{
		"../../skills/lark-im/references/lark-im-messages-send.md",
		"../../skills/lark-im/references/lark-im-messages-reply.md",
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, line := range strings.Split(string(source), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "lark-cli im images create ") {
				continue
			}
			count++
			if strings.Contains(line, " --as ") {
				t.Errorf("dual-identity images.create skill example must preserve identity choice in %s: %s", path, line)
			}
		}
		if count != 2 {
			t.Errorf("images.create examples in %s = %d, want 2 audited upload steps", path, count)
		}
	}

	image := parsedIMAffordance(t, "images.create")
	if len(image.Examples) != 1 || strings.Contains(image.Examples[0].Command, " --as ") {
		t.Fatalf("images.create affordance must preserve the caller's user/bot identity choice: %#v", image.Examples)
	}
	for _, useWhen := range image.UseWhen {
		if strings.Contains(strings.ToLower(useWhen), "bot-only") {
			t.Fatalf("images.create affordance must not repeat the stale bot-only description: %v", image.UseWhen)
		}
	}
}

func TestIMImageUploadMetadataSupportsBothIdentities(t *testing.T) {
	if len(registry.EmbeddedServicesTyped()) == 0 {
		t.Skip("generated API metadata is not embedded in this bare-module test run")
	}
	target, err := registry.EmbeddedCatalog().Resolve([]string{"im", "images", "create"})
	if err != nil {
		t.Fatal(err)
	}
	if target.Method == nil {
		t.Fatal("im.images.create resolved without a method")
	}
	method := target.Method.Method
	if !method.SupportsToken(meta.TokenUser) || !method.SupportsToken(meta.TokenTenant) {
		t.Fatalf("im.images.create accessTokens = %v, want both user and tenant", method.AccessTokens)
	}
}

func parsedIMAffordance(t *testing.T, method string) meta.Affordance {
	t.Helper()
	raw, ok := For("im", method)
	if !ok {
		t.Fatalf("For(im, %s) ok=false", method)
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatalf("im %s affordance did not parse", method)
	}
	return a
}

func containsExact(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func compactSkillText(value string) string {
	value = strings.ReplaceAll(value, "\\\r\n", "")
	value = strings.ReplaceAll(value, "\\\n", "")
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}
