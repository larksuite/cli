// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

type publishBudgetInfo struct {
	fileio.FileInfo
	size int64
}

func (s publishBudgetInfo) Size() int64 { return s.size }

type publishBudgetFileIO struct {
	fileio.FileIO
	sizes  map[string]int64
	opened []string
}

func (f *publishBudgetFileIO) Stat(name string) (fileio.FileInfo, error) {
	st, err := f.FileIO.Stat(name)
	if err != nil {
		return nil, err
	}
	if size, ok := f.sizes[filepath.Clean(name)]; ok {
		return publishBudgetInfo{FileInfo: st, size: size}, nil
	}
	return st, nil
}
func (f *publishBudgetFileIO) Open(name string) (fileio.File, error) {
	f.opened = append(f.opened, filepath.Clean(name))
	return f.FileIO.Open(name)
}

type publishBudgetProvider struct{ fio fileio.FileIO }

func (publishBudgetProvider) Name() string                                  { return "publish-budget-test" }
func (p publishBudgetProvider) ResolveFileIO(context.Context) fileio.FileIO { return p.fio }

func TestPublish_RejectsOversizedDependenciesBeforeReading(t *testing.T) {
	for _, mode := range []string{"Publish", "Dry_Run"} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				html     string
				sizes    map[string]int64
				rejected string
			}{
				{"Image", `<img src="a.png">`, map[string]int64{"a.png": maxPublishBytes + 1}, "a.png"},
				{"Script", `<script src="a.js"></script>`, map[string]int64{"a.js": maxPublishBytes + 1}, "a.js"},
				{"Cumulative", `<img src="a.png"><img src="b.png">`, map[string]int64{"a.png": maxPublishBytes / 2, "b.png": maxPublishBytes / 2}, "b.png"},
				{"Transitive", `<link rel="stylesheet" href="style.css">`, map[string]int64{"a.png": maxPublishBytes}, "a.png"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cmdutil.TestChdir(t, t.TempDir())
					t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
					writeFile(t, "page.html", []byte(tc.html))
					writeFile(t, "style.css", []byte(`body { background:url(a.png) }`))
					for name := range tc.sizes {
						writeFile(t, name, []byte("x"))
					}
					if tc.name == "Cumulative" {
						asset, err := os.OpenFile("a.png", os.O_WRONLY, 0)
						require.NoError(t, err)
						require.NoError(t, asset.Truncate(maxPublishBytes/2))
						require.NoError(t, asset.Close())
					}

					f, out, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-html", AppSecret: "test-secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"})
					fio := &publishBudgetFileIO{FileIO: f.ResolveFileIO(context.Background()), sizes: tc.sizes}
					f.FileIOProvider = publishBudgetProvider{fio: fio}
					stub := successPublishStub()
					stub.Optional = true
					reg.Register(stub)
					parent := &cobra.Command{Use: "html", SilenceErrors: true, SilenceUsage: true}
					Publish.Mount(parent, f)
					args := []string{"+publish", "--file-path", "page.html", "--as", "user"}
					if mode == "Dry_Run" {
						args = append(args, "--dry-run")
					}
					parent.SetArgs(args)
					err := parent.ExecuteContext(context.Background())
					require.Error(t, err)
					p, ok := errs.ProblemOf(err)
					require.True(t, ok)
					assert.Equal(t, errs.CategoryValidation, p.Category)
					assert.Equal(t, errs.SubtypeFailedPrecondition, p.Subtype)
					var validation *errs.ValidationError
					require.ErrorAs(t, err, &validation)
					assert.Equal(t, "--file-path", validation.Param)
					assert.NotContains(t, fio.opened, tc.rejected)
					assert.Empty(t, stub.CapturedBodies)
					assert.Empty(t, out.String())
				})
			}
		})
	}
}
