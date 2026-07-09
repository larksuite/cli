// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"sort"
	"strings"

	rootcmd "github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/registry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func collectHandAuthored(ctx context.Context) (manifest.Manifest, error) {
	root := rootcmd.Build(ctx, cmdutil.InvocationContext{},
		rootcmd.WithIO(strings.NewReader(""), io.Discard, io.Discard),
		rootcmd.WithoutPlugins(),
		rootcmd.WithoutStrictMode(),
		rootcmd.WithoutServiceCommands(),
	)

	return collectFromRoot(root), nil
}

func collectCommandIndex(ctx context.Context) (manifest.Manifest, error) {
	root := rootcmd.Build(ctx, cmdutil.InvocationContext{},
		rootcmd.WithIO(strings.NewReader(""), io.Discard, io.Discard),
		rootcmd.WithoutPlugins(),
		rootcmd.WithoutStrictMode(),
		rootcmd.WithServiceCatalog(registry.EmbeddedCatalog()),
	)

	idx := collectFromRoot(root)
	handAuthored, err := collectHandAuthored(ctx)
	if err != nil {
		return manifest.Manifest{}, err
	}
	return overlayHandAuthoredCommands(idx, handAuthored), nil
}

func collectFromRoot(root *cobra.Command) manifest.Manifest {
	var commands []manifest.Command
	walkCommands(root, func(c *cobra.Command) {
		if c == root {
			return
		}
		commands = append(commands, commandFromCobra(c, nil))
	})
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Path < commands[j].Path
	})

	return manifest.Manifest{SchemaVersion: 1, Commands: commands}
}

func overlayHandAuthoredCommands(idx, handAuthored manifest.Manifest) manifest.Manifest {
	byPath := make(map[string]manifest.Command, len(handAuthored.Commands))
	for _, cmd := range handAuthored.Commands {
		byPath[cmd.Path] = cmd
	}
	for i, cmd := range idx.Commands {
		if handCmd, ok := byPath[cmd.Path]; ok {
			idx.Commands[i] = handCmd
		}
	}
	return idx
}

func walkCommands(root *cobra.Command, visit func(*cobra.Command)) {
	visit(root)
	for _, child := range root.Commands() {
		walkCommands(child, visit)
	}
}

func commandFromCobra(c *cobra.Command, defaultFields map[string][]string) manifest.Command {
	path := strings.TrimPrefix(c.CommandPath(), "lark-cli ")
	source := manifest.SourceBuiltin
	if s, ok := cmdmeta.SourceOf(c); ok {
		source = manifest.Source(s)
	}
	entry := manifest.Command{
		Path:          path,
		CanonicalPath: manifest.CanonicalCommandPath(path),
		Domain:        commandDomain(c, path, source),
		Use:           c.Use,
		Short:         c.Short,
		Example:       c.Example,
		Hidden:        c.Hidden,
		Runnable:      c.Runnable(),
		Source:        source,
		Generated:     cmdmeta.Generated(c),
		Identities:    cmdmeta.Identities(c),
		DefaultFields: defaultFields[path],
	}
	if risk, ok := cmdmeta.Risk(c); ok {
		entry.Risk = risk
	}

	c.Flags().VisitAll(func(f *pflag.Flag) {
		entry.Flags = append(entry.Flags, flagFromPFlag(f))
	})
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if findFlag(entry.Flags, f.Name) == nil {
			entry.Flags = append(entry.Flags, flagFromPFlag(f))
		}
	})
	sort.Slice(entry.Flags, func(i, j int) bool {
		return entry.Flags[i].Name < entry.Flags[j].Name
	})
	return entry
}

func commandDomain(c *cobra.Command, path string, source manifest.Source) string {
	if domain := cmdmeta.Domain(c); domain != "" {
		return domain
	}
	if source == manifest.SourceService {
		if first, _, ok := strings.Cut(path, " "); ok {
			return first
		}
		return path
	}
	return ""
}

func flagFromPFlag(f *pflag.Flag) manifest.Flag {
	return manifest.Flag{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Usage:       f.Usage,
		Hidden:      f.Hidden,
		Required:    hasAnnotation(f, cobra.BashCompOneRequiredFlag),
		TakesValue:  f.NoOptDefVal == "",
		DefValue:    f.DefValue,
		NoOptValue:  f.NoOptDefVal,
		Annotations: cloneAnnotations(f.Annotations),
	}
}

func findFlag(flags []manifest.Flag, name string) *manifest.Flag {
	for i := range flags {
		if flags[i].Name == name {
			return &flags[i]
		}
	}
	return nil
}

func hasAnnotation(f *pflag.Flag, key string) bool {
	if f.Annotations == nil {
		return false
	}
	values, ok := f.Annotations[key]
	return ok && len(values) > 0
}

func cloneAnnotations(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}
