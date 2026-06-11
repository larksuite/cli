# Base Formula Examples and Requirement Translation

> 本文件是 [formula-field-guide.md](formula-field-guide.md) 的按需补充：完整示例与"自然语言需求 → 公式"的翻译规则。

## Section 13: Complete Examples

### Example 1: Employee sales summary

**Table structure** (from `+table-get`):

- Employees: EmployeeID (Text), Name (Text), Department (Text)
- Sales: ContractID (Number), SalespersonID (Text), Quantity (Number), Total (Number)

**Current table**: Employees

**Requirement**: For each employee, output "Sold XX orders" if they have sales records, otherwise "No sales records".

**Formula**:

```
IF(
  [Sales].COUNTIF(CurrentValue.[SalespersonID] = [EmployeeID]) >= 1,
  "Sold " & [Sales].COUNTIF(CurrentValue.[SalespersonID] = [EmployeeID]) & " orders",
  "No sales records"
)
```

**Field JSON**:

```json
{
  "type": "formula",
  "name": "Sales Summary",
  "expression": "IF([Sales].COUNTIF(CurrentValue.[SalespersonID] = [EmployeeID]) >= 1, \"Sold \" & [Sales].COUNTIF(CurrentValue.[SalespersonID] = [EmployeeID]) & \" orders\", \"No sales records\")"
}
```

**Explanation**: `[Sales].COUNTIF(...)` uses the entire Sales table as data range. CurrentValue represents each row in Sales, accessing `CurrentValue.[SalespersonID]` for that row's salesperson. `[EmployeeID]` refers to the current row in the Employees table (where the formula lives).

### Example 2: Chained cross-table access via link fields

**Table structure**:

- Orders: ID (`auto_number`), OrderItems (`link` [target: OrderItems, foreign key: ID])
- OrderItems: ID (`auto_number`), Product (`link` [target: Products, foreign key: ID])
- Products: ID (`auto_number`), ProductName (`text`)

**Current table**: Orders

**Requirement**: Deduplicate and comma-join all product names from linked order items.

**Formula**:

```
[OrderItems].[Product].[ProductName].UNIQUE().ARRAYJOIN(",")
```

**Field JSON**:

```json
{
  "type": "formula",
  "name": "Product List",
  "expression": "[OrderItems].[Product].[ProductName].UNIQUE().ARRAYJOIN(\",\")"
}
```

**Explanation**: `[OrderItems]` gets linked order item records, `.[Product]` expands to each item's linked product, `.[ProductName]` gets all product names, `.UNIQUE()` deduplicates, `.ARRAYJOIN(",")` joins with commas.

### Example 3: Cross-table filter + sort

**Table structure**:

- Projects: ProjectName (Text), Status (Text), Owner (Text)
- Tasks: TaskName (Text), Project (Text), Priority (Number), DueDate (Date)

**Current table**: Projects

**Requirement**: Find the highest-priority (lowest number) task name for the current project.

**Formula**:

```
FIRST(
  [Tasks].FILTER(CurrentValue.[Project] = [ProjectName]).SORTBY([Tasks].[Priority], TRUE).[TaskName]
)
```

**Field JSON**:

```json
{
  "type": "formula",
  "name": "Top Priority Task",
  "expression": "FIRST([Tasks].FILTER(CurrentValue.[Project] = [ProjectName]).SORTBY([Tasks].[Priority], TRUE).[TaskName])"
}
```

**Explanation**: `[Tasks].FILTER(CurrentValue.[Project] = [ProjectName])` filters tasks belonging to the current project. `.SORTBY([Tasks].[Priority], TRUE)` sorts by priority ascending. `.[TaskName]` extracts task names. `FIRST(...)` gets the first one (highest priority).

---

## Section 14: Translating User Requirements to Formulas

When the user describes their formula need in natural language, follow these rules to convert it into a precise expression:

1. **Numbers must use precise values**: "less than 80%" → field value less than `0.8`. "above 1000" → `>= 1000`.
2. **Interval boundaries**: "above/below/within" = closed (inclusive); "less than/more than/outside" = open (exclusive).
3. **Branching logic** must be organized as an ordered list with a fallback branch. Each branch has a condition and output.
   - Example: "return risk level for 1-3" → `IFS([Value] = 1, "low", [Value] = 2, "medium", [Value] = 3, "high")` with an `IFERROR` or trailing empty-string fallback.
4. **Multi-level branches must be flattened** to a single level. Nested if-else chains → flat IFS.
5. **Branch conditions must be mutually exclusive**. If the user's conditions overlap, rewrite to eliminate ambiguity.
6. **Reorder branches by logical priority** if the user's order is illogical (e.g., check specific conditions before catch-all).

---
