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

// contentFlagAliases are the spellings agents reach for instead of --content
// when handing a whole page of XML to +update-slide.
//
// Deliberately not "slide": several slides commands take a --slide-id, so
// `--slide <id>` is a likely typo for that, and resolving it to --content
// would turn the typo into a request carrying an id where page XML belongs.
var contentFlagAliases = []string{
	"xml",
	"slide-xml",
	"slide-content",
	"content-xml",
}

// presentationAliasMap resolves every --presentation spelling and is attached
// to every shortcut that declares that flag.
var presentationAliasMap = aliasMap(map[string][]string{"presentation": presentationFlagAliases})

// wholePageAliasMap additionally resolves the --content spellings. It is
// attached only to the whole-page overwrite commands: --content exists on
// other slides shortcuts too, and letting these aliases resolve there would
// rewrite a mistyped flag into one the caller never meant to use.
var wholePageAliasMap = aliasMap(map[string][]string{
	"presentation": presentationFlagAliases,
	"content":      contentFlagAliases,
})

// aliasMap inverts canonical→aliases into alias→canonical.
func aliasMap(byCanonical map[string][]string) map[string]string {
	out := make(map[string]string)
	for canonical, aliases := range byCanonical {
		for _, alias := range aliases {
			out[alias] = canonical
		}
	}
	return out
}

// Shortcuts returns all slides shortcuts.
func Shortcuts() []common.Shortcut {
	all := []struct {
		shortcut common.Shortcut
		aliases  map[string]string
	}{
		{shortcut: SlidesCreate, aliases: presentationAliasMap},
		{shortcut: SlidesMediaUpload, aliases: presentationAliasMap},
		{shortcut: SlidesReplaceSlide, aliases: presentationAliasMap},
		{shortcut: SlidesReplacePages, aliases: presentationAliasMap},
		{shortcut: SlidesUpdateSlide, aliases: wholePageAliasMap},
		{shortcut: SlidesUpdate, aliases: wholePageAliasMap},
		{shortcut: SlidesScreenshot, aliases: presentationAliasMap},
		{shortcut: SlidesXMLGet, aliases: presentationAliasMap},
		{shortcut: SlidesHistoryList, aliases: presentationAliasMap},
		{shortcut: SlidesHistoryRevert, aliases: presentationAliasMap},
		{shortcut: SlidesHistoryRevertStatus, aliases: presentationAliasMap},
	}
	out := make([]common.Shortcut, 0, len(all))
	for _, entry := range all {
		if hasAliasableFlag(entry.shortcut.Flags, entry.aliases) {
			entry.shortcut.PostMount = withFlagAliases(entry.aliases, entry.shortcut.PostMount)
		}
		out = append(out, entry.shortcut)
	}
	return out
}

// hasAliasableFlag reports whether the shortcut declares a flag that one of
// the aliases resolves to, i.e. whether attaching the normalizer can do
// anything.
func hasAliasableFlag(flags []common.Flag, aliases map[string]string) bool {
	for _, flag := range flags {
		for _, canonical := range aliases {
			if flag.Name == canonical {
				return true
			}
		}
	}
	return false
}

// withFlagAliases accepts common agent-generated spellings for canonical flags
// without registering extra flags. The aliases therefore stay out of help and
// completion while resolving to the canonical flag at parse time, matching the
// zero-round-trip compatibility used by Sheets.
func withFlagAliases(aliases map[string]string, prev func(cmd *cobra.Command)) func(cmd *cobra.Command) {
	return func(cmd *cobra.Command) {
		if prev != nil {
			prev(cmd)
		}
		cmd.Flags().SetNormalizeFunc(func(fs *pflag.FlagSet, name string) pflag.NormalizedName {
			// fs.Lookup re-enters this func with the canonical name; that
			// terminates because no canonical name is itself an alias key
			// (asserted by TestFlagAliasesAreNotCanonicalNames). Looking the
			// canonical name up keeps a mistyped alias reported as the flag
			// the caller actually typed on commands that lack the target.
			if canonical, ok := aliases[name]; ok && fs.Lookup(canonical) != nil {
				return pflag.NormalizedName(canonical)
			}
			return pflag.NormalizedName(name)
		})
	}
}
