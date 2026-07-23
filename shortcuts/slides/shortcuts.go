// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var presentationFlagAliases = []string{
	"presentation-id",
	"presentation-token",
	"token",
	"presentation_id",
	"xml-presentation-id",
	"url",
}

// Shortcuts returns all slides shortcuts.
func Shortcuts() []common.Shortcut {
	all := []common.Shortcut{
		SlidesCreate,
		SlidesMediaUpload,
		SlidesReplaceSlide,
		SlidesReplacePages,
		SlidesScreenshot,
		SlidesXMLGet,
		SlidesHistoryList,
		SlidesHistoryRevert,
		SlidesHistoryRevertStatus,
	}
	for i := range all {
		if hasPresentationFlag(all[i].Flags) {
			all[i].PostMount = withPresentationFlagAliases(all[i].PostMount)
		}
	}
	return all
}

func hasPresentationFlag(flags []common.Flag) bool {
	for _, flag := range flags {
		if flag.Name == "presentation" {
			return true
		}
	}
	return false
}

// withPresentationFlagAliases accepts common agent-generated spellings for
// --presentation without registering extra flags. The aliases therefore stay
// out of help and completion while resolving to the canonical flag at parse
// time, matching the zero-round-trip compatibility used by Sheets.
func withPresentationFlagAliases(prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}
		cmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
			for _, alias := range presentationFlagAliases {
				if name == alias {
					return pflag.NormalizedName("presentation")
				}
			}
			return pflag.NormalizedName(name)
		})
	}
}
