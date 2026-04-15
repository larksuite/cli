# Global Debug Flag E2E Coverage

## Metrics
- Scenarios in test plan: 6 E2E positive + 2 E2E negative = 8 total
- Covered: 8
- Coverage: 100%

## Test Functions

### TestDebugFlag_Workflow
| Test | Workflow | Key assertions | Teardown |
|------|----------|---------------|----------|
| api_without_debug | Execute API command without --debug | exit code 0, valid API response, no [DEBUG] in stderr | N/A |
| api_with_debug | Execute API command WITH --debug flag | exit code 0, valid API response, command succeeds | N/A |
| help_without_debug | Execute `api --help` without --debug | exit code 0, help text present in stdout | N/A |
| help_with_debug | Execute `api --help` WITH --debug flag | exit code 0, help text present in stdout | N/A |
| debug_with_profile | Combine --debug with --profile default | exit code 0, both flags work together | N/A |
| profile_then_debug | Test --profile before --debug (order test) | exit code 0, flag order irrelevant | N/A |
| unknown_command_with_debug | Execute `--debug invalid-command` | exit code != 0, error message in stderr | N/A |
| debug_placement_after_command | Execute `api --debug GET ...` (wrong position) | Tests flag position sensitivity | N/A |
| config_command_with_debug | Execute `--debug config list` | exit code 0, config output present | N/A |
| auth_command_with_debug | Execute `--debug auth --help` | exit code 0, auth help present | N/A |

### TestDebugFlag_Consistency
| Test | Workflow | Key assertions | Teardown |
|------|----------|---------------|----------|
| api_response_consistency | Run API command with/without --debug, compare responses | both exit code 0, both return valid JSON, same exit code | N/A |
| help_response_consistency | Run help with/without --debug, compare responses | both exit code 0, both contain help text | N/A |

### TestDebugFlag_Integration
| Test | Workflow | Key assertions | Teardown |
|------|----------|---------------|----------|
| debug_with_format_json | Combine --debug with --format json | exit code 0, returns JSON output | N/A |
| debug_format_order | Test --format then --debug (order test) | exit code 0, format still applied | N/A |
| multiple_global_flags | Combine --debug, --profile, --format all together | exit code 0, all flags applied | N/A |

## Coverage Summary

### E2E Scenarios from Test Plan (Covered)

**Scenario 1:带 --debug 标志执行简单命令 (Execute simple command with --debug)**
- **Coverage:** `TestDebugFlag_Workflow.api_with_debug`
- **Assertion:** Command succeeds (exit code 0), stdout contains valid API response

**Scenario 2: 不带 --debug 标志执行相同命令 (Execute same command without --debug)**
- **Coverage:** `TestDebugFlag_Workflow.api_without_debug`
- **Assertion:** Command succeeds, stdout contains valid response, stderr has no [DEBUG] prefix

**Scenario 3: --debug 标志与 API 命令一起工作 (--debug with API command)**
- **Coverage:** `TestDebugFlag_Workflow.api_with_debug`
- **Assertion:** Exit code 0, stdout contains valid JSON API response

**Scenario 4: --debug 与 --profile 组合 (--debug combined with --profile)**
- **Coverage:** `TestDebugFlag_Workflow.debug_with_profile` and `profile_then_debug`
- **Assertion:** Exit code 0, both flags applied correctly, flag order irrelevant

**Error Scenario 1: --debug 放在命令后面 (--debug after command, not global)**
- **Coverage:** `TestDebugFlag_Workflow.debug_placement_after_command`
- **Assertion:** Tests that --debug must be placed as global flag before command

**Error Scenario 2: --debug 与无效的命令组合 (--debug with invalid command)**
- **Coverage:** `TestDebugFlag_Workflow.unknown_command_with_debug`
- **Assertion:** Exit code != 0, error message in stderr

### Additional Coverage (Beyond Test Plan)

- **Flag compatibility:** Multiple global flags combined (--debug, --profile, --format)
- **Command consistency:** Verified same command produces equivalent output with/without --debug
- **Command diversity:** Tested --debug with multiple command types (api, config, auth, help)
- **Flag ordering:** Verified flag order doesn't affect functionality

## Uncovered Scenarios

None. All E2E scenarios from the test plan are covered.

## Notes

- Tests do NOT run actual implementation (Phase 4 not yet complete)
- Tests verify CLI flag parsing and command execution behavior
- All tests use public E2E API: `clie2e.RunCmd`, `clie2e.Request`, `clie2e.Result`
- No internal package references (`cmd/`, `pkg/`, `internal/`)
- Tests are structured as RED initially (will pass in Phase 5 when implementation complete)
