// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package busdiscover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// psCommand is the PowerShell one-liner that emits JSON for bus candidate
// processes. We filter by Name='lark-cli.exe' in WQL to avoid enumerating
// the entire process table. .ToString('o') produces ISO 8601 with fractional
// seconds and timezone — stable across locales.
//
// PowerShell 5.1 compatibility: we deliberately do NOT use `-AsArray`, which
// was introduced in PowerShell 6.0 (Core). Without it, ConvertTo-Json emits
// a JSON array for 2+ results and a bare object for a single result; 0
// results produces empty output. parseWindowsPS handles all three shapes.
//
// WQL filter limitation: this matches only the standard `lark-cli.exe`
// binary name (as distributed by npm and GitHub releases). Renamed binaries
// will not be detected — unlike the Unix scanner, which matches any cmdline
// containing "lark-cli". Documented asymmetry; acceptable trade-off for the
// performance win of avoiding a full process-table enumeration.
const psCommand = `Get-CimInstance Win32_Process -Filter "Name='lark-cli.exe'" | ` +
	`ForEach-Object { [pscustomobject]@{ Pid = $_.ProcessId; ` +
	`CreationDate = $_.CreationDate.ToString('o'); ` +
	`CommandLine = $_.CommandLine } } | ConvertTo-Json`

func newPlatformScanner() Scanner {
	return &windowsScanner{
		runPS: func() ([]byte, error) {
			cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", psCommand)
			return cmd.Output()
		},
	}
}

// windowsScanner shells out to PowerShell's Get-CimInstance Win32_Process.
// runPS is a field for test injection.
type windowsScanner struct {
	runPS func() ([]byte, error)
}

// psRecord matches the JSON shape emitted by our PowerShell command.
type psRecord struct {
	Pid          int    `json:"Pid"`
	CreationDate string `json:"CreationDate"`
	CommandLine  string `json:"CommandLine"`
}

func (s *windowsScanner) ScanBusProcesses() ([]Process, error) {
	out, err := s.runPS()
	if err != nil {
		// When PowerShell exits non-zero, the real diagnostic (parameter
		// errors, missing CIM class, unavailable WinRM) lives in Stderr.
		// Surface it — `exit status 1` alone is never actionable.
		if ee, ok := err.(*exec.ExitError); ok && len(bytes.TrimSpace(ee.Stderr)) > 0 {
			return nil, fmt.Errorf("busdiscover: run powershell: %w: %s", err, bytes.TrimSpace(ee.Stderr))
		}
		return nil, fmt.Errorf("busdiscover: run powershell: %w", err)
	}
	return parseWindowsPS(out)
}

// parseWindowsPS decodes PowerShell JSON output and filters to bus processes.
// ConvertTo-Json (without -AsArray, which is PS 6.0+) emits:
//   - empty output for 0 results
//   - a bare `{...}` object for 1 result
//   - a `[...]` array for 2+ results
//
// We handle all three shapes. An unexpected first byte surfaces as an error
// so we can debug environments where PowerShell behaves differently.
func parseWindowsPS(out []byte) ([]Process, error) {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return nil, nil
	}
	var records []psRecord
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("busdiscover: decode ps json array: %w", err)
		}
	case '{':
		var single psRecord
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, fmt.Errorf("busdiscover: decode ps json object: %w", err)
		}
		records = []psRecord{single}
	default:
		return nil, fmt.Errorf("busdiscover: unexpected ps output: %q", trimmed[:min(len(trimmed), 64)])
	}

	var result []Process
	for _, r := range records {
		appID := parseAppIDFromCmdline(r.CommandLine)
		if appID == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, r.CreationDate)
		if err != nil {
			// Skip entries with unparseable timestamps rather than failing
			// the whole scan — one bad row shouldn't hide other buses.
			continue
		}
		result = append(result, Process{
			PID:       r.Pid,
			AppID:     appID,
			StartTime: t,
		})
	}
	return result, nil
}
