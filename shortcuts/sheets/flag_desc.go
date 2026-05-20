// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/larksuite/cli/shortcuts/common"
)

//go:embed data/flag-descriptions.en.json
var flagDescsJSON []byte

var (
	flagDescsOnce sync.Once
	flagDescs     map[string]map[string]string
	flagDescsErr  error
)

func loadFlagDescs() (map[string]map[string]string, error) {
	flagDescsOnce.Do(func() {
		flagDescs = make(map[string]map[string]string)
		flagDescsErr = json.Unmarshal(flagDescsJSON, &flagDescs)
		if flagDescsErr != nil {
			flagDescsErr = fmt.Errorf("flag-descriptions.en.json: %w", flagDescsErr)
		}
	})
	return flagDescs, flagDescsErr
}

// flagDesc returns the description for a flag from the embedded
// flag-descriptions.en.json. command is e.g. "+workbook-info",
// flagName is e.g. "url" (without "--" prefix). Returns "" when
// no entry exists.
func flagDesc(command, flagName string) string {
	descs, err := loadFlagDescs()
	if err != nil || descs == nil {
		return ""
	}
	cmd, ok := descs[command]
	if !ok {
		return ""
	}
	return cmd["--"+flagName]
}

// applyFlagDescs patches all Flag.Desc fields in the given shortcut
// slice with values from flag-descriptions.en.json. Flags without a
// JSON entry keep their existing Desc unchanged.
func applyFlagDescs(shortcuts []common.Shortcut) {
	descs, err := loadFlagDescs()
	if err != nil || descs == nil {
		return
	}
	for i := range shortcuts {
		cmd, ok := descs[shortcuts[i].Command]
		if !ok {
			continue
		}
		for j := range shortcuts[i].Flags {
			key := "--" + shortcuts[i].Flags[j].Name
			if desc, found := cmd[key]; found {
				shortcuts[i].Flags[j].Desc = desc
			}
		}
	}
}
