// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

const larkBaseSkillDoc = "../../skills/lark-base/SKILL.md"

func readSkillContractFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill doc %s: %v", path, err)
	}
	return string(raw)
}

func TestBaseSkillContract_ReusedBlockConfigPreservesExplicitIntent(t *testing.T) {
	doc := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(doc, "- BaseApp（应用模式）")
	end := strings.Index(doc, "- 应用页面的 block")
	if start < 0 || end <= start {
		t.Fatalf("missing BaseApp routing section in %s", larkBaseSkillDoc)
	}
	section := doc[start:end]
	for _, contract := range []string{
		"只能作为结构模板",
		"首次 Create/Update 前仍要逐项对齐用户显式要求",
		"`group_by[].sort.order`",
		"顶层 `sort.order`",
		"不能用旧配置省略的方向",
		"`get-data` 结果顺序代替",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("BaseApp routing must contain %q:\n%s", contract, section)
		}
	}
}

func TestBaseSkillContract_AppModeConceptsAndDataConfigRelationship(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	for _, contract := range []string{
		"## 应用模式与 Workspace 心智模型",
		"Workspace 是组织 Base 和 BaseApp 的空间容器",
		"Workspace 负责资源归属，App 负责页面和组件，Base 负责数据",
	} {
		if !strings.Contains(skill, contract) {
			t.Fatalf("Base skill must contain %q", contract)
		}
	}

	appConfig := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-app-block-data-config.md")
	for _, contract := range []string{
		"复用 [Dashboard Block 配置](lark-base-dashboard-block-config.md)",
		"列表组件是 App 独有协议",
		"所有列表 subtype 均可使用",
		"不能把 `filter` 提到顶层",
	} {
		if !strings.Contains(appConfig, contract) {
			t.Fatalf("App block config reference must contain %q", contract)
		}
	}

	dashboardConfig := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-dashboard-block-config.md")
	for _, contract := range []string{
		"复用本文的字段取值、筛选、分组、排序及规范化规则",
		"`isGreaterEqual` / `isLessEqual` 不是全局不支持",
		"可用于 `number`，但不能用于 `datetime`",
	} {
		if !strings.Contains(dashboardConfig, contract) {
			t.Fatalf("Dashboard block config reference must contain %q", contract)
		}
	}
}

func TestBaseSkillContract_FormulaGuideBeforeUnsupportedFallback(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(skill, "### Field")
	end := strings.Index(skill, "### Record")
	if start < 0 || end <= start {
		t.Fatalf("missing Field routing section in %s", larkBaseSkillDoc)
	}
	fieldSection := skill[start:end]
	for _, contract := range []string{
		"[Formula guide](references/lark-base-field-formula.md)",
		"明确请求 Formula 创建或更新时",
		"说明不支持或改用其他字段类型前",
		"`[SourceTable].[NumericField]` 是 List",
		"`SUM([SourceTable].[NumericField])`",
	} {
		if !strings.Contains(fieldSection, contract) {
			t.Fatalf("Formula routing must contain %q:\n%s", contract, fieldSection)
		}
	}

	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	if !strings.Contains(formulaGuide, "returns `tables[].name`") {
		t.Fatalf("Formula guide must describe the actual +table-list response shape")
	}
	if strings.Contains(formulaGuide, "items[].table_name") {
		t.Fatalf("Formula guide must not advertise the obsolete +table-list response shape")
	}
}

func TestBaseSkillContract_FormulaActionRequestsExecuteAndReadBack(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(skill, "### Field")
	end := strings.Index(skill, "### Record")
	if start < 0 || end <= start {
		t.Fatalf("missing Field routing section in %s", larkBaseSkillDoc)
	}
	fieldSection := skill[start:end]
	for _, contract := range []string{
		"用户已提供 Base 并明确要求创建或更新 Formula 字段时",
		"指定结果写入的目标表并要求用 Formula 产出结果时",
		"即使未使用“创建/更新”字样",
		"除非用户明确只要解释",
		"先完整阅读 [Formula guide](references/lark-base-field-formula.md)",
		"完成表/字段发现后",
		"按用户语句中的语法角色区分写入目标和引用来源",
		"`+field-create` / `+field-update`",
		"`+field-get`",
		"`+record-list`",
		"只给公式建议不算完成",
	} {
		if !strings.Contains(fieldSection, contract) {
			t.Fatalf("Formula action routing must contain %q:\n%s", contract, fieldSection)
		}
	}

	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	for _, contract := range []string{
		"## Action requests and operand roles",
		"names the destination table and asks Formula to produce a result",
		"even if the request does not literally say create or update",
		"destination table",
		"source table and field",
		"Do not swap these roles",
		"read back the final Formula field with `+field-get`",
		"read a representative computed value with `+record-list`",
	} {
		if !strings.Contains(formulaGuide, contract) {
			t.Fatalf("Formula guide must contain %q", contract)
		}
	}
}

func TestBaseSkillContract_FormulaPredicatesPreserveRequestedSemantics(t *testing.T) {
	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	for _, contract := range []string{
		"## Preserve requested predicate semantics",
		"Treat `equals`, `is`, and `is one of` as exact comparisons by default",
		"Do not add `TRIM`, `LOWER`, `UPPER`, `CONTAINTEXT`, `CONTAIN`, regex, fuzzy matching, or synonym expansion",
		"Only normalize case or whitespace when the user explicitly requests it",
		"Only use containment when the user requests contains or membership semantics",
		"the schema proves that the searched operand is a list",
		"Compare the candidate expression back to the requested predicate before mutation",
		"readback does not make a semantically broader expression acceptable",
	} {
		if !strings.Contains(formulaGuide, contract) {
			t.Fatalf("Formula predicate preservation must contain %q", contract)
		}
	}
}

func TestBaseSkillContract_FormulaTransformationsPreserveRequestedSemantics(t *testing.T) {
	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	for _, contract := range []string{
		"## Preserve requested transformation semantics",
		"Do not add sorting, first-occurrence ordering, `TRIM`, case folding, whitespace cleanup, or other normalization",
		"An unmentioned comparison column can reveal an ambiguity, but it does not authorize replacing an expression that already satisfies the request",
		"`UNIQUE` guarantees deduplication only",
		"its output order is engine-defined and is not a first-occurrence guarantee",
		"Use plain `UNIQUE` when the user asks only to deduplicate",
		"Use the `TRIM` mapping only when the user explicitly requests whitespace cleanup",
		"Apply the same check to the final `+field-get` expression",
	} {
		if !strings.Contains(formulaGuide, contract) {
			t.Fatalf("Formula transformation preservation must contain %q", contract)
		}
	}
	usage := strings.Index(formulaGuide, "## Usage")
	transformation := strings.Index(formulaGuide, "## Preserve requested transformation semantics")
	if transformation < 0 || usage < 0 || transformation >= usage {
		t.Fatal("Formula transformation invariant must appear before Usage")
	}

	skill := readSkillContractFile(t, larkBaseSkillDoc)
	for _, contract := range []string{
		"Formula 变换必须保持用户明确要求的语义",
		"不要自行增加排序、首次出现顺序、`TRIM` 或大小写/空格归一化",
		"未被用户点名的样例列或结果列只能暴露歧义",
	} {
		if !strings.Contains(skill, contract) {
			t.Fatalf("Base skill Formula transformation invariant must contain %q", contract)
		}
	}
}

func TestBaseSkillContract_FormulaBranchesPreserveFallbacks(t *testing.T) {
	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	for _, contract := range []string{
		"## Preserve complete branch semantics",
		"truth table that includes every requested condition, `otherwise` / fallback result, and blank or null boundary",
		"Current sample rows do not authorize dropping an unobserved branch",
		"when checking duplicates for an optional identifier",
		"a blank value means no identifier",
		"unless the user explicitly asks to treat blank values as duplicate keys",
		"back-translate every branch of the final `+field-get` expression into the truth table",
		"verify branches absent from the current records from the expression structure itself",
	} {
		if !strings.Contains(formulaGuide, contract) {
			t.Fatalf("Formula branch completeness must contain %q", contract)
		}
	}
	usage := strings.Index(formulaGuide, "## Usage")
	branches := strings.Index(formulaGuide, "## Preserve complete branch semantics")
	if branches < 0 || usage < 0 || branches >= usage {
		t.Fatal("Formula branch invariant must appear before Usage")
	}

	skill := readSkillContractFile(t, larkBaseSkillDoc)
	for _, contract := range []string{
		"Formula 条件分支必须先列出完整真值表",
		"保留每个条件、`otherwise` / fallback 与空值边界",
		"可选标识符为空时",
		"走未重复或既定 fallback",
		"从最终 `+field-get` 表达式结构确认该分支存在",
	} {
		if !strings.Contains(skill, contract) {
			t.Fatalf("Base skill Formula branch invariant must contain %q", contract)
		}
	}
	for _, forbidden := range []string{"base_formula_", "grading_pass_rate", "larkoffice.com/base/"} {
		if strings.Contains(strings.ToLower(formulaGuide), forbidden) || strings.Contains(strings.ToLower(skill), forbidden) {
			t.Fatalf("Formula branch guidance must remain generic, found %q", forbidden)
		}
	}
}

func TestBaseSkillContract_FormulaDateDifferenceHandlesEitherOrdering(t *testing.T) {
	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	for _, contract := range []string{
		"## Preserve formula operand precision",
		"A `datetime` operand includes its time component",
		"“difference in days” or a result unit of days does not by itself request calendar-day truncation or an integer",
		"Do not add `TEXT`, `TODATE`, `DATE`, `INT`, `ROUND`, `ROUNDDOWN`, or `ROUNDUP`",
		"Field style, display formatting, and current sample rows do not authorize loss of precision",
		"back-translate direction, sign, and precision / granularity",
		"#### Choose a date-difference function by semantics",
		"For a general difference in days, use `DAYS(end, start)`",
		"For elapsed days from a date through today, use `DAYS(TODAY(), date)`",
		"When the result must be non-negative, use `ABS(DAYS(end, start))`",
		"Use `DATEDIF` only when the user requests whole elapsed days, months, or years",
		"the start date is guaranteed not to be after the end date",
		"Back-check the expression against both past and future dates",
		"Rows that currently exercise only one date ordering do not prove the formula handles the other ordering",
		"Apply the same semantic check to the final `+field-get` expression",
		"### Mistake 10: Ignoring DAYS parameter order",
		"A negative result is not inherently wrong",
		"Choose argument order from the requested direction",
	} {
		if !strings.Contains(formulaGuide, contract) {
			t.Fatalf("Formula date-difference rules must contain %q", contract)
		}
	}
	for _, forbidden := range []string{
		"Wrong:   DAYS([StartDate], [EndDate])",
		"Correct: DAYS([EndDate], [StartDate])",
	} {
		if strings.Contains(formulaGuide, forbidden) {
			t.Fatalf("Formula date-difference rules must not contain %q", forbidden)
		}
	}
	usage := strings.Index(formulaGuide, "## Usage")
	precision := strings.Index(formulaGuide, "## Preserve formula operand precision")
	if precision < 0 || usage < 0 || precision >= usage {
		t.Fatal("Formula precision invariant must appear before Usage")
	}
	if !strings.Contains(formulaGuide, "for explicit day-level equality only; do not reuse it to truncate operands in date arithmetic") {
		t.Fatal("Formula equality conversion guidance must not leak into date arithmetic")
	}

	skill := readSkillContractFile(t, larkBaseSkillDoc)
	for _, contract := range []string{
		"Formula 日期差必须同时保留方向、符号和精度",
		"`datetime` 字段默认以完整值直接参与日期算术",
		"字段 style、显示格式和当前样例值不授权降精度",
	} {
		if !strings.Contains(skill, contract) {
			t.Fatalf("Base skill Formula precision invariant must contain %q", contract)
		}
	}
}

func TestBaseSkillContract_BaseURLsStayOnCLIAuthBoundary(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(skill, "## 进入前必做：解析目标实体")
	end := strings.Index(skill, "## Base 模板中心")
	if start < 0 || end <= start {
		t.Fatalf("missing Base target resolution section in %s", larkBaseSkillDoc)
	}
	section := skill[start:end]
	for _, contract := range []string{
		"`/base/` 和 `/app/` 目标",
		"`lark-cli base +url-resolve`",
		"不得使用通用文档读取器（包括 `GetDocument`）",
		"通用 connector 的授权失败与 `lark-cli` 用户授权相互独立",
		"不能据此判定 `lark-cli` 无权限",
		"只有实际 `lark-cli` 命令返回的认证或授权错误",
		"`lark-cli skills read lark-base <relative-path>`",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("Base routing must contain %q:\n%s", contract, section)
		}
	}
}

func TestBaseSkillContract_ConcreteMutationCannotStopAtPreparation(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(skill, "# Base")
	end := strings.Index(skill, "## 身份选择（优先）")
	if start < 0 || end <= start {
		t.Fatalf("missing early Base execution contract in %s", larkBaseSkillDoc)
	}
	section := skill[start:end]
	for _, contract := range []string{
		"## 行动请求（优先）",
		"已提供具体 Base 或 BaseApp 目标",
		"创建、更新、删除、复制、移动、配置、提交或生成在线对象/数据",
		"先按对应对象协议确认请求能力受支持",
		"对受支持的行动请求，除非用户明确只要解释、示例、命令或 JSON 文本，或明确要求不执行，否则必须执行相应的 `lark-cli` 命令",
		"上述非执行请求保持非变更：只返回用户要求的内容，不得执行写命令",
		"对需要执行的请求，读取 guide、构造表达式、JSON 或命令都只是准备步骤，不能作为成功终态",
		"按对象协议检查操作结果和必要的最终状态",
		"未执行所需写命令且未证明目标当前已满足时不得报告完成",
		"协议不支持的请求按对应 reference 的能力边界停止，不得用其他对象或写入冒充",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("early Base execution contract must contain %q:\n%s", contract, section)
		}
	}
}

func TestBaseSkillContract_ExplicitLookupPreservesTypeAndValidatesDependencies(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(skill, "### Field")
	end := strings.Index(skill, "### Record")
	if start < 0 || end <= start {
		t.Fatalf("missing Field routing section in %s", larkBaseSkillDoc)
	}
	fieldSection := skill[start:end]
	for _, contract := range []string{
		"[Lookup guide](references/lark-base-field-lookup.md)",
		"明确请求 Lookup 时",
		"`type=lookup` 是输出契约",
		"除非用户明确批准变更字段类型",
		"空值、暂未计算或配置错误只触发排查",
		"不得改用 Formula、Link 或其他字段类型",
	} {
		if !strings.Contains(fieldSection, contract) {
			t.Fatalf("Lookup routing must contain %q:\n%s", contract, fieldSection)
		}
	}

	lookupGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-lookup.md")
	for _, contract := range []string{
		"returns `tables[].name`",
		"`meta.pagination.complete` is `false`",
		"pass its decimal `next_token` to `--offset`",
		"`type=lookup` is an invariant",
		"empty or temporarily uncomputed result",
		"diagnose the requested Lookup",
		"topological order",
		"Use a separate command for each dependency layer",
		"only independent fields in the same layer may be batched",
		"read-modify-write",
		"confirm `type`, `from`, `select`, `where`, and `aggregate`",
		"before creating or updating any dependent Lookup",
		"read every requested field again with `+field-get`",
		"read representative computed values with `+record-list`",
		"`[无效引用]`",
		"`Invalid Reference`",
		"blocks completion",
	} {
		if !strings.Contains(lookupGuide, contract) {
			t.Fatalf("Lookup guide must contain %q", contract)
		}
	}
	if strings.Contains(lookupGuide, "items[].table_name") {
		t.Fatalf("Lookup guide must not advertise the obsolete +table-list response shape")
	}
}

func TestBaseSkillContract_LookupAggregateRequiresExplicitDeduplication(t *testing.T) {
	lookupGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-lookup.md")
	start := strings.Index(lookupGuide, "## Section 4: Aggregate Rules")
	end := strings.Index(lookupGuide, "## Section 5: Hard Constraints")
	if start < 0 || end <= start {
		t.Fatal("missing Lookup aggregate rules section")
	}
	section := lookupGuide[start:end]
	for _, contract := range []string{
		"Use `raw_value` for list, reference, or bring-through intent",
		"unless the user explicitly asks to deduplicate",
		"`unique` is allowed only for explicit deduplicate, distinct, or unique intent",
		"collection, list, set, or similar nouns in a destination label do not authorize `unique`",
		"Preserve an explicitly requested `sum`, `average`, `max`, `min`, `counta`, or `unique_counta`",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("Lookup aggregate rules must contain %q:\n%s", contract, section)
		}
	}
	if !strings.Contains(lookupGuide, "Just reference matching values? → Lookup (aggregate = raw_value)") {
		t.Fatal("Lookup decision tree must keep plain references on raw_value")
	}
	if strings.Contains(lookupGuide, "aggregate = null") {
		t.Fatal("Lookup guide must not route plain references to a null aggregate")
	}
}

func TestBaseSkillContract_LookupCountPreservesCountedOperand(t *testing.T) {
	lookupGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-lookup.md")
	start := strings.Index(lookupGuide, "## Section 4: Aggregate Rules")
	end := strings.Index(lookupGuide, "## Section 5: Hard Constraints")
	if start < 0 || end <= start {
		t.Fatal("missing Lookup aggregate rules section")
	}
	section := lookupGuide[start:end]
	for _, contract := range []string{
		"Resolve the counted noun before choosing `select` and `aggregate`",
		"`select=F` with `aggregate=counta` counts non-empty occurrences of `F`",
		"`select=N` with `aggregate=sum` adds the numeric values of `N`",
		"Explicitly named additive `number` field",
		"select `F` and use `counta`",
		"prefer a schema-confirmed stable, non-empty identifier",
		"an explicitly named ID or `auto_number` field",
		"Only fall back to a primary / display field when no stronger identifier exists",
		"Do not replace an entity count with `sum` of a numeric measure",
		"A bare “total” or “total number” label does not authorize `sum`",
		"Do not pick an arbitrary non-empty field for `counta`",
		"Sample totals that happen to match do not establish semantic equivalence",
		"clarify the counted operand before mutation",
		"Apply the same back-translation to the final `+field-get` readback",
		"representative values that happen to match do not make it acceptable",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("Lookup count rules must contain %q:\n%s", contract, section)
		}
	}
	for _, forbidden := range []string{
		"| `sum` | \"total\"",
		"| Any field |",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("Lookup count rules must not contain %q:\n%s", forbidden, section)
		}
	}
}

func TestBaseSkillContract_LookupNamedCountedFieldCannotBecomeRecordCount(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	for _, contract := range []string{
		"查找引用计数必须先绑定“被统计对象”",
		"用户点名某个来源字段的出现次数时，`select` 必须是该字段且 `aggregate=counta`",
		"只有明确统计记录/实体且未点名被统计字段时",
		"匹配字段和 `select` 可以是同一字段",
		"不能为了“数记录”改选其他非空字段",
	} {
		if !strings.Contains(skill, contract) {
			t.Fatalf("Base skill Lookup count invariant must contain %q", contract)
		}
	}

	lookupGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-lookup.md")
	for _, contract := range []string{
		"Do not reframe a named field-occurrence request as a record count",
		"The filter field and `select` may be the same source field",
		"An identifier fallback is allowed only for an entity or record count with no named counted field",
		"State the mapping as `counted operand -> select -> aggregate` before mutation",
	} {
		if !strings.Contains(lookupGuide, contract) {
			t.Fatalf("Lookup named-counted-field rules must contain %q", contract)
		}
	}
}

func TestBaseSkillContract_LookupCorrelationFollowsCurrentRowSemantics(t *testing.T) {
	lookupGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-lookup.md")
	start := strings.Index(lookupGuide, "### How to find the matching field pair")
	end := strings.Index(lookupGuide, "### Where condition structure")
	if start < 0 || end <= start {
		t.Fatal("missing Lookup row-correlation section")
	}
	section := lookupGuide[start:end]
	for _, contract := range []string{
		"When multiple schema-valid field pairs exist",
		"derive the correlation from the user's relationship and the entity represented by the current row",
		"For a list or aggregate computed per current entity",
		"prefer a schema-confirmed source identifier or Link to that entity",
		"`select` controls the returned value; it is not the join key",
		"both tables expose it or sample values happen to coincide",
		"An explicitly requested alternate relationship or key wins",
		"Names alone are insufficient",
		"semantic meaning, Link targets, and comparison compatibility",
		"request, schemas, and dependent Lookups",
		"ask one targeted clarification before writing",
		"Back-translate the final `where`",
		"Sample values are secondary",
		"Source links to current entity → source.linkField matches current.primaryField",
		"Current row links to source    → source.primaryField matches current.linkField",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("Lookup row-correlation rules must contain %q:\n%s", contract, section)
		}
	}
}

func TestBaseSkillContract_ComputedFieldConvertibilityDoesNotOverrideExplicitType(t *testing.T) {
	updateGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-update.md")
	for _, contract := range []string{
		"转换白名单只描述技术可转换性，不授权改变用户明确要求的字段类型",
		"用户明确要求 Formula 或 Lookup 语义时，必须保留该类型",
		"除非用户明确要求转换",
	} {
		if !strings.Contains(updateGuide, contract) {
			t.Fatalf("field-update guide must contain %q", contract)
		}
	}
	if strings.Contains(updateGuide, "无状态字段可直接转换") {
		t.Fatal("field-update guide must not present computed-field convertibility as unconditional authorization")
	}

	guides := []struct {
		name          string
		path          string
		invariant     string
		conversionBan string
	}{
		{
			name:          "formula",
			path:          "../../skills/lark-base/references/lark-base-field-formula.md",
			invariant:     "`type=formula` is an invariant",
			conversionBan: "Technical convertibility to Lookup does not authorize a type change",
		},
		{
			name:          "lookup",
			path:          "../../skills/lark-base/references/lark-base-field-lookup.md",
			invariant:     "`type=lookup` is an invariant",
			conversionBan: "Technical convertibility to Formula does not authorize a type change",
		},
	}
	for _, guide := range guides {
		t.Run(guide.name, func(t *testing.T) {
			doc := readSkillContractFile(t, guide.path)
			for _, contract := range []string{
				guide.invariant,
				guide.conversionBan,
				"unless the user explicitly requests conversion",
			} {
				if !strings.Contains(doc, contract) {
					t.Fatalf("%s guide must contain %q", guide.name, contract)
				}
			}
		})
	}
}

func TestBaseSkillContract_ComputedFieldVerificationPollsWithoutReplayingMutation(t *testing.T) {
	guides := []struct {
		name string
		path string
	}{
		{name: "formula", path: "../../skills/lark-base/references/lark-base-field-formula.md"},
		{name: "lookup", path: "../../skills/lark-base/references/lark-base-field-lookup.md"},
	}
	for _, guide := range guides {
		t.Run(guide.name, func(t *testing.T) {
			doc := readSkillContractFile(t, guide.path)
			for _, contract := range []string{
				"Poll read-only commands only",
				"same Base, table, and field",
				"bounded retries with backoff and a hard deadline",
				"A stale read must never trigger replay",
				"`+field-create` or `+field-update`",
				"deadline expires, verification has failed",
			} {
				if !strings.Contains(doc, contract) {
					t.Fatalf("%s guide must contain %q", guide.name, contract)
				}
			}
		})
	}
}

func TestBaseSkillContract_FormulaTableDiscoveryConsumesAllPages(t *testing.T) {
	formulaGuide := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-field-formula.md")
	for _, contract := range []string{
		"When `meta.pagination.complete` is `false`",
		"pass its decimal `next_token` to `--offset`",
		"repeat until `complete` is `true`",
		"Do not assume the default first page contains the destination or source table",
	} {
		if !strings.Contains(formulaGuide, contract) {
			t.Fatalf("Formula table discovery must contain %q", contract)
		}
	}
}
