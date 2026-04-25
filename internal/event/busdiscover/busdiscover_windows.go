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

// psCommand omits -AsArray (PS 6.0+) so PS 5.1 works; parseWindowsPS handles 0/1/N shapes.
// Matches only lark-cli.exe by name — renamed binaries are not detected (asymmetric vs unix).
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

type windowsScanner struct {
	runPS func() ([]byte, error)
}

type psRecord struct {
	Pid          int    `json:"Pid"`
	CreationDate string `json:"CreationDate"`
	CommandLine  string `json:"CommandLine"`
}

func (s *windowsScanner) ScanBusProcesses() ([]Process, error) {
	out, err := s.runPS()
	if err != nil {
		// Surface PowerShell stderr — `exit status 1` alone is not actionable.
		if ee, ok := err.(*exec.ExitError); ok && len(bytes.TrimSpace(ee.Stderr)) > 0 {
			return nil, fmt.Errorf("busdiscover: run powershell: %w: %s", err, bytes.TrimSpace(ee.Stderr))
		}
		return nil, fmt.Errorf("busdiscover: run powershell: %w", err)
	}
	return parseWindowsPS(out)
}

// parseWindowsPS handles 0/1/N ConvertTo-Json shapes (empty / object / array).
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
			continue // one bad row shouldn't hide other buses
		}
		result = append(result, Process{
			PID:       r.Pid,
			AppID:     appID,
			StartTime: t,
		})
	}
	return result, nil
}
