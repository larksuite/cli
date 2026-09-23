// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"os"
	"strings"
	"testing"
)

func TestTaskSkillUsesRuntimeDiscoveryInsteadOfStaticMetaInventory(t *testing.T) {
	raw, err := os.ReadFile("../../skills/lark-task/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)

	for _, required := range []string{
		"--print-schema --flag-name <flag>",
		"字段不在 schema 中",
		"重新选择 shortcut",
		"lark-cli schema task.<resource>.<method>",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("lark-task discovery guidance missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"## API Resources",
		"## 权限表",
		"### sections",
		"### custom_fields",
		"### custom_field_options",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("lark-task duplicates Meta-owned inventory %q", forbidden)
		}
	}
}

func TestTaskSkillAssignsRelationshipOwnershipBeforeCompositeData(t *testing.T) {
	raw, err := os.ReadFile("../../skills/lark-task/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)

	start := strings.Index(content, "## 字段与关系所有权")
	if start < 0 {
		t.Fatal("lark-task skill is missing the field and relationship ownership section")
	}
	section := content[start:]
	if next := strings.Index(section[len("## "):], "\n## "); next >= 0 {
		section = section[:len("## ")+next]
	}
	for _, required := range []string{
		"对已有任务的负责人",
		"必须使用 [`+assign`]",
		"创建任务时可直接使用 `+create --assignee`",
		"同一条 `+assign` 命令中使用 `--remove <old>` 和 `--add <new>`",
		"禁止把 `members` 传入 `+update --data`",
		"禁止从查询结果反推其他更新参数",
	} {
		if !strings.Contains(section, required) {
			t.Errorf("relationship ownership guidance missing %q: %s", required, section)
		}
	}
}

func TestTaskUpdateReferenceDiscoversSchemaBeforeDataExample(t *testing.T) {
	raw, err := os.ReadFile("../../skills/lark-task/references/lark-task-update.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)

	schemaCommand := "lark-cli task +update --print-schema --flag-name data"
	schemaIndex := strings.Index(content, schemaCommand)
	if schemaIndex < 0 {
		t.Fatalf("update reference is missing schema discovery command %q", schemaCommand)
	}

	const commandPrefix = "lark-cli task +update"
	dataExamples := 0
	for searchFrom := 0; searchFrom < len(content); {
		relativeStart := strings.Index(content[searchFrom:], commandPrefix)
		if relativeStart < 0 {
			break
		}
		commandStart := searchFrom + relativeStart
		commandEnd := commandStart
		for {
			relativeEnd := strings.IndexByte(content[commandEnd:], '\n')
			if relativeEnd < 0 {
				commandEnd = len(content)
				break
			}
			commandEnd += relativeEnd
			if !strings.HasSuffix(strings.TrimSpace(content[commandStart:commandEnd]), "\\") {
				break
			}
			commandEnd++
		}

		command := content[commandStart:commandEnd]
		if strings.Contains(command, "--data") {
			dataExamples++
			if commandStart < schemaIndex {
				t.Errorf("schema discovery must precede every --data example: schema=%d command=%d: %s", schemaIndex, commandStart, command)
			}
		}
		searchFrom = commandStart + len(commandPrefix)
	}
	if dataExamples == 0 {
		t.Fatal("update reference is missing an executable +update --data example")
	}
}
