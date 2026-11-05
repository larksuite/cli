// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package payload

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

type budgetFileInfo struct {
	fileio.FileInfo
	size int64
}

func (s budgetFileInfo) Size() int64 { return s.size }

type budgetReadProbe struct {
	*bytes.Reader
	consumed int64
	closed   bool
	readErr  error
}

func (p *budgetReadProbe) Read(b []byte) (int, error) {
	n, err := p.Reader.Read(b)
	p.consumed += int64(n)
	if p.readErr != nil {
		return n, p.readErr
	}
	return n, err
}
func (p *budgetReadProbe) Close() error { p.closed = true; return nil }

type budgetFileIO struct {
	permissiveFIO
	sizes  map[string]int64
	probes map[string]*budgetReadProbe
	opened []string
}

func (f *budgetFileIO) Stat(name string) (fileio.FileInfo, error) {
	st, err := f.permissiveFIO.Stat(name)
	if err != nil {
		return nil, err
	}
	if size, ok := f.sizes[name]; ok {
		return budgetFileInfo{FileInfo: st, size: size}, nil
	}
	return st, nil
}
func (f *budgetFileIO) Open(name string) (fileio.File, error) {
	f.opened = append(f.opened, name)
	if p := f.probes[name]; p != nil {
		return p, nil
	}
	return f.permissiveFIO.Open(name)
}

func TestCollectFileWithBudget_RejectsBeforeOpen(t *testing.T) {
	for _, tc := range []struct {
		name   string
		entry  string
		first  int64
		second int64
	}{
		{"Oversized_Image", `<img src="a.png">`, 101, 0},
		{"Oversized_Script", `<script src="a.js"></script>`, 101, 0},
		{"Cumulative_Assets", `<img src="a.png"><img src="b.png">`, 40, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			entry := filepath.Join(root, "page.html")
			asset := filepath.Join(root, "a.png")
			if tc.name == "Oversized_Script" {
				asset = filepath.Join(root, "a.js")
			}
			second := filepath.Join(root, "b.png")
			mustWrite(t, entry, tc.entry)
			mustWrite(t, asset, strings.Repeat("x", int(tc.first)))
			mustWrite(t, second, "x")
			sizes := map[string]int64{asset: tc.first}
			if tc.second != 0 {
				sizes[second] = tc.second
			}
			fio := &budgetFileIO{sizes: sizes}
			cands, key, skipped, err := CollectFileWithBudget(fio, entry, 100)
			ve := requireValidation(t, err, errs.SubtypeFailedPrecondition)
			assert.Equal(t, "--file-path", ve.Param)
			assert.Nil(t, cands)
			assert.Empty(t, key)
			assert.Nil(t, skipped)
			if tc.second == 0 {
				assert.NotContains(t, fio.opened, asset)
			} else {
				assert.NotContains(t, fio.opened, second)
			}
		})
	}
}

func TestCollectFileWithBudget_BoundsActualReads(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  string
		readErr error
	}{
		{"Entry", "page.html", nil},
		{"Image", "a.png", nil},
		{"Script", "a.js", nil},
		{"Entry_Partial_Read_Error", "page.html", io.ErrUnexpectedEOF},
		{"Image_Partial_Read_Error", "a.png", io.ErrUnexpectedEOF},
		{"Script_Partial_Read_Error", "a.js", io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			const entryContent = `<img src="a.png"><script src="a.js"></script>`
			entry := filepath.Join(root, "page.html")
			mustWrite(t, entry, entryContent)
			mustWrite(t, filepath.Join(root, "a.png"), "x")
			mustWrite(t, filepath.Join(root, "a.js"), "x")
			targetPath := filepath.Join(root, tc.target)
			probe := &budgetReadProbe{Reader: bytes.NewReader(bytes.Repeat([]byte("x"), 1024*1024)), readErr: tc.readErr}
			fio := &budgetFileIO{sizes: map[string]int64{targetPath: 1}, probes: map[string]*budgetReadProbe{targetPath: probe}}
			_, _, _, err := CollectFileWithBudget(fio, entry, 100)
			requireValidation(t, err, errs.SubtypeFailedPrecondition)
			prior := int64(0)
			if tc.target != "page.html" {
				prior = int64(len(entryContent))
			}
			if tc.target == "a.js" {
				prior++
			}
			assert.Equal(t, 100-prior+1, probe.consumed, "read only the remaining budget and one overflow byte")
			assert.True(t, probe.closed)
		})
	}
}

func TestCollectFileWithBudget_ExactBoundaryAndDeduplication(t *testing.T) {
	root := t.TempDir()
	const html = `<img src="a.png"><img src="a.png">`
	const image = "1234567"
	entry := filepath.Join(root, "page.html")
	asset := filepath.Join(root, "a.png")
	mustWrite(t, entry, html)
	mustWrite(t, asset, image)
	fio := &budgetFileIO{}
	cands, key, skipped, err := CollectFileWithBudget(fio, entry, int64(len(html)+len(image)))
	require.NoError(t, err)
	require.Len(t, cands, 2)
	assert.Equal(t, int64(len(image)), cands[1].Size)
	assert.NotEmpty(t, key)
	assert.Empty(t, skipped)
	assert.Equal(t, []string{entry, asset}, fio.opened)
}

func TestCollectFileWithBudget_ChargesFailedDependencyReads(t *testing.T) {
	root := t.TempDir()
	const html = `<img src="a.png"><img src="b.png">`
	entry := filepath.Join(root, "page.html")
	first := filepath.Join(root, "a.png")
	second := filepath.Join(root, "b.png")
	mustWrite(t, entry, html)
	mustWrite(t, first, "x")
	mustWrite(t, second, "12345678901")
	probe := &budgetReadProbe{Reader: bytes.NewReader(bytes.Repeat([]byte("x"), 20)), readErr: io.ErrUnexpectedEOF}
	fio := &budgetFileIO{probes: map[string]*budgetReadProbe{first: probe}}
	_, _, _, err := CollectFileWithBudget(fio, entry, int64(len(html)+30))
	requireValidation(t, err, errs.SubtypeFailedPrecondition)
	assert.Equal(t, int64(20), probe.consumed)
	assert.True(t, probe.closed)
	assert.NotContains(t, fio.opened, second)
}

func TestCollectFile_DefaultBudget(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "page.html")
	asset := filepath.Join(root, "a.png")
	mustWrite(t, entry, `<img src="a.png">`)
	mustWrite(t, asset, "x")
	fio := &budgetFileIO{sizes: map[string]int64{asset: DefaultLimits().RawTotalBytes + 1}}
	_, _, _, err := CollectFile(fio, entry)
	requireValidation(t, err, errs.SubtypeFailedPrecondition)
	assert.NotContains(t, fio.opened, asset)
}
