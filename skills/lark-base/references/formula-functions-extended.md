# Base Formula Functions — Extended (rare functions)

> 本文件是 [formula-field-guide.md](formula-field-guide.md) Section 8 的长尾补充：三角/双曲/随机数/进制/统计扩展等罕见函数。
> 表格列含义与主文档一致：Function | Signature | Return type | Description。

## 8.1 Logic functions (extended)

| Function | Signature | Return type | Description |
|---|---|---|---|
| ISERROR | `ISERROR(expr)` | Boolean | Tests if expression errors |
| ISNUMBER | `ISNUMBER(value)` | Boolean | Tests if value is a number |
| CONTAINSALL | `CONTAINSALL(search_range, value, ...)` | Boolean | Tests if a list or `select` (`multiple=true`) contains all specified values |
| CONTAINSONLY | `CONTAINSONLY(search_range, value, ...)` | Boolean | Tests if a list or `select` (`multiple=true`) contains only the specified values |
| TRUE | `TRUE()` | Boolean | Returns TRUE |
| FALSE | `FALSE()` | Boolean | Returns FALSE |
| RANDOMBETWEEN | `RANDOMBETWEEN(min_int, max_int, [keep_updating])` | Number | Random integer in the specified range |
| RANDOMITEM | `RANDOMITEM(list, [keep_updating])` | Matches element type | Randomly picks one element from a list |

## 8.2 Numeric functions (extended)

| Function | Signature | Return type | Description |
|---|---|---|---|
| MEDIAN | `MEDIAN(val1, val2, ...)` | Number | Median |
| ROUNDUP | `ROUNDUP(number, digits)` | Number | Round away from zero. Same digits semantics as ROUND |
| ROUNDDOWN | `ROUNDDOWN(number, digits)` | Number | Round toward zero. Same digits semantics as ROUND |
| FLOOR | `FLOOR(number, [base])` | Number | Round down to nearest multiple of base (default 1) |
| CEILING | `CEILING(number, [base])` | Number | Round up to nearest multiple of base (default 1) |
| POWER | `POWER(base, exponent)` | Number | Exponentiation |
| QUOTIENT | `QUOTIENT(dividend, divisor)` | Number | Integer division |
| ISODD | `ISODD(number)` | Boolean | Tests if number is odd |
| RANK | `RANK(value, search_range, [ascending])` | Number | Rank of value in range; default descending |
| SEQUENCE | `SEQUENCE(start, end, [step])` | List | Generate number sequence |
| PI | `PI()` | Number | Pi constant |
| SIN/COS/TAN/ASIN/ACOS/ATAN/ATAN2/SINH/COSH/TANH/ASINH/ACOSH/ATANH | `func(radians_or_value)` | Number | Trigonometric and hyperbolic functions; arguments in radians |

## 8.3 Text functions (extended)

| Function | Signature | Return type | Description |
|---|---|---|---|
| FIND | `FIND(search_val, search_range, [start])` | Number | Find substring position (case-sensitive); returns -1 if not found |
| TODATE | `TODATE(value)` | Date | Convert date string to date type |
| CHAR | `CHAR(number)` | Text | ASCII code to character |
| FORMAT | `FORMAT(template, [val1, val2, ...])` | Text | Template string formatting; use `{1}`, `{2}` as placeholders |
| HYPERLINK | `HYPERLINK(url, [display_text])` | Hyperlink | Create a hyperlink |
| ENCODEURL | `ENCODEURL(text)` | Text | URL encode |
| REGEXMATCH | `REGEXMATCH(text, regex)` | Boolean | Regex match test |
| REGEXEXTRACT | `REGEXEXTRACT(text, regex)` | List | Extract first match's capture groups |
| REGEXEXTRACTALL | `REGEXEXTRACTALL(text, regex)` | 2D List | Extract all matches |
| REGEXREPLACE | `REGEXREPLACE(text, regex, replacement)` | Text | Regex replace |

## 8.4 Date functions (extended)

| Function | Signature | Return type | Description |
|---|---|---|---|
| DURATION | `DURATION(days, [hours], [minutes], [seconds])` | Duration | Create a duration for date arithmetic |
| EDATE | `EDATE(date, months)` | Date | Date N months later |
| EOMONTH | `EOMONTH(date, [months])` | Date | End of month N months later; months default 0 |
| WORKDAY | `WORKDAY(start_date, days, [holidays])` | Date | Date N workdays later (skips weekends and holidays) |

## 8.5 List functions (extended)

| Function | Signature | Return type | Description |
|---|---|---|---|
| LIST | `LIST(val1, val2, ...)` | List | Create a list |
| NTH | `NTH(list, index)` | Scalar | Nth element (1-based) |
| DISTANCE | `DISTANCE(location1, location2)` | Number | Distance between two geographic locations (km) |
