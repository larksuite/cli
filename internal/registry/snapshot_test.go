// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var expectedSnapshotServices = []string{
	"approval",
	"attendance",
	"calendar",
	"contact",
	"drive",
	"im",
	"mail",
	"mindnotes",
	"minutes",
	"okr",
	"sheets",
	"slides",
	"task",
	"vc",
	"wiki",
}

func TestOpenSnapshotFSRejectsInvalidManifest(t *testing.T) {
	base := validSnapshotMapFS(t, "drive")
	tests := []struct {
		name   string
		mutate func(fstest.MapFS)
		reason string
	}{
		{
			name: "unknown schema version",
			mutate: func(fsys fstest.MapFS) {
				rewriteManifest(t, fsys, func(m map[string]any) { m["schema_version"] = 2 })
			},
			reason: "unsupported schema_version 2",
		},
		{
			name: "invalid schema version type",
			mutate: func(fsys fstest.MapFS) {
				rewriteManifest(t, fsys, func(m map[string]any) { m["schema_version"] = "1" })
			},
			reason: "invalid JSON",
		},
		{
			name: "no services",
			mutate: func(fsys fstest.MapFS) {
				rewriteManifest(t, fsys, func(m map[string]any) { m["services"] = []any{} })
			},
			reason: "services must be a non-empty sorted array",
		},
		{
			name: "unsorted services",
			mutate: func(fsys fstest.MapFS) {
				second := manifestEntryFor(t, "calendar", fsys["services/drive.json"].Data)
				rewriteManifest(t, fsys, func(m map[string]any) {
					m["services"] = []any{m["services"].([]any)[0], second}
				})
				fsys["services/calendar.json"] = &fstest.MapFile{Data: slices.Clone(fsys["services/drive.json"].Data)}
			},
			reason: "services must be a non-empty sorted array",
		},
		{
			name: "duplicate service",
			mutate: func(fsys fstest.MapFS) {
				rewriteManifest(t, fsys, func(m map[string]any) {
					entry := m["services"].([]any)[0]
					m["services"] = []any{entry, entry}
				})
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "invalid name",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) {
					entry["name"] = "../drive"
					entry["file"] = "services/../drive.json"
				})
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "non-fixed path",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) { entry["file"] = "services/calendar.json" })
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "extra entry field",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) { entry["unexpected"] = true })
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "invalid revision",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) { entry["revision"] = 0 })
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "invalid size",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) { entry["size"] = 0 })
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "invalid sha256",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) { entry["sha256"] = strings.Repeat("A", 64) })
			},
			reason: "invalid or duplicate service entry",
		},
		{
			name: "missing file",
			mutate: func(fsys fstest.MapFS) {
				delete(fsys, "services/drive.json")
			},
			reason: "service file set does not match manifest",
		},
		{
			name: "extra file",
			mutate: func(fsys fstest.MapFS) {
				fsys["services/extra.json"] = &fstest.MapFile{Data: []byte(`{}`)}
			},
			reason: "service file set does not match manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := cloneMapFS(base)
			tt.mutate(fsys)
			_, err := OpenSnapshotFS(fsys)
			requireCatalogIntegrityError(t, err, "embedded catalog manifest is invalid: "+tt.reason)
		})
	}
}

func TestOpenSnapshotFSAcceptsSchemaV1ManifestWithoutSourceSHA256(t *testing.T) {
	fsys := validSnapshotMapFS(t, "drive")

	snapshot, err := OpenSnapshotFS(fsys)
	require.NoError(t, err)
	assert.Equal(t, []string{"drive"}, snapshot.ServiceNames())
}

func TestOpenSnapshotFSAcceptsServiceNamePrefixWithHyphen(t *testing.T) {
	fsys := validSnapshotMapFS(t, "task")
	taskXBody := []byte(`{"name":"task-x","version":"v1","servicePath":"/open-apis/task-x","resources":{}}`)
	fsys["services/task-x.json"] = &fstest.MapFile{Data: taskXBody}
	rewriteManifest(t, fsys, func(manifest map[string]any) {
		manifest["services"] = append(
			manifest["services"].([]any),
			manifestEntryFor(t, "task-x", taskXBody),
		)
	})

	snapshot, err := OpenSnapshotFS(fsys)
	require.NoError(t, err)
	assert.Equal(t, []string{"task", "task-x"}, snapshot.ServiceNames())
}

func TestOpenSnapshotFSReportsManifestReadFailure(t *testing.T) {
	cause := errors.New("manifest storage is unavailable")
	_, err := OpenSnapshotFS(&failingFS{
		FS:     validSnapshotMapFS(t, "drive"),
		target: "manifest.json",
		err:    cause,
	})

	requireCatalogIntegrityError(t, err, "embedded catalog manifest could not be read")
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, cause, errors.Unwrap(err))
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.Equal(t, "run lark-cli update to restore the embedded catalog", problem.Hint)
}

func TestOpenSnapshotReportsFilesystemOpenFailure(t *testing.T) {
	cause := errors.New("catalog subtree is unavailable")
	_, err := openSnapshot(&failingSubFS{
		FS:  validSnapshotMapFS(t, "drive"),
		err: cause,
	}, "catalog")

	requireCatalogIntegrityError(t, err, "embedded catalog filesystem could not be opened")
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, cause, errors.Unwrap(err))
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.Equal(t, "run lark-cli update to restore the embedded catalog", problem.Hint)
}

func TestCatalogIntegrityValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(fstest.MapFS)
		reason string
	}{
		{
			name: "size mismatch",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) { entry["size"] = 1 })
			},
			reason: "size mismatch",
		},
		{
			name: "sha256 mismatch",
			mutate: func(fsys fstest.MapFS) {
				rewriteEntry(t, fsys, func(entry map[string]any) {
					entry["sha256"] = strings.Repeat("0", 64)
				})
			},
			reason: "sha256 mismatch",
		},
		{
			name: "invalid JSON",
			mutate: func(fsys fstest.MapFS) {
				body := []byte(`{"name":`)
				fsys["services/drive.json"] = &fstest.MapFile{Data: body}
				rewriteEntryForBody(t, fsys, body)
			},
			reason: "invalid JSON",
		},
		{
			name: "service name mismatch",
			mutate: func(fsys fstest.MapFS) {
				body := []byte(`{"name":"calendar","version":"v1","servicePath":"/open-apis/calendar","resources":{}}`)
				fsys["services/drive.json"] = &fstest.MapFile{Data: body}
				rewriteEntryForBody(t, fsys, body)
			},
			reason: "service name mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := validSnapshotMapFS(t, "drive")
			tt.mutate(fsys)
			snapshot, err := OpenSnapshotFS(fsys)
			require.NoError(t, err)
			err = snapshot.Catalog().Preload("drive")
			requireCatalogIntegrityError(t, err, `embedded catalog service "drive" failed integrity validation: `+tt.reason)
			_, err = snapshot.Load("drive")
			requireCatalogIntegrityError(t, err, `embedded catalog service "drive" failed integrity validation: `+tt.reason)
		})
	}
}

func TestCatalogIntegrityNamesCRLFCheckout(t *testing.T) {
	fsys := validSnapshotMapFS(t, "drive")
	lf := []byte("{\n  \"name\": \"drive\",\n  \"version\": \"v1\",\n" +
		"  \"servicePath\": \"/open-apis/drive\",\n  \"resources\": {}\n}\n")
	fsys["services/drive.json"] = &fstest.MapFile{Data: lf}
	rewriteEntryForBody(t, fsys, lf)
	fsys["services/drive.json"] = &fstest.MapFile{
		Data: bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n")),
	}

	snapshot, err := OpenSnapshotFS(fsys)
	require.NoError(t, err)
	_, err = snapshot.Load("drive")

	requireCatalogIntegrityError(t, err, `embedded catalog service "drive" failed integrity validation: size mismatch`)
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.Contains(t, problem.Hint, "CRLF line endings")
}

// TestCatalogIntegritySizeMismatchWithoutCRLFSaysNothingAboutLineEndings keeps
// the diagnosis honest: a shard that is simply the wrong size must not be
// blamed on a checkout filter.
func TestCatalogIntegritySizeMismatchWithoutCRLFSaysNothingAboutLineEndings(t *testing.T) {
	fsys := validSnapshotMapFS(t, "drive")
	rewriteEntry(t, fsys, func(entry map[string]any) { entry["size"] = 1 })

	snapshot, err := OpenSnapshotFS(fsys)
	require.NoError(t, err)
	_, err = snapshot.Load("drive")

	requireCatalogIntegrityError(t, err, `embedded catalog service "drive" failed integrity validation: size mismatch`)
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.NotContains(t, problem.Hint, "CRLF")
}

// TestGitAttributesPinsCatalogLineEndings guards the repository-level half of
// the same defect: without an eol=lf attribute a Windows checkout under
// core.autocrlf rewrites every shard and fails the digest above.
func TestGitAttributesPinsCatalogLineEndings(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".gitattributes"))
	require.NoError(t, err, ".gitattributes must exist so catalog shards stay LF on every platform")
	assert.Contains(t, string(data), "eol=lf")
}

func TestOpenSnapshotFSIsLazyAndCatalogReadsEachShardOnce(t *testing.T) {
	base := validSnapshotMapFS(t, "drive")
	rewriteManifest(t, base, func(manifest map[string]any) {
		manifest["future_optional_field"] = map[string]any{"accepted": true}
	})
	counting := &countingFS{FS: base, opens: make(map[string]int)}

	snapshot, err := OpenSnapshotFS(counting)
	require.NoError(t, err)
	assert.Zero(t, counting.opens["services/drive.json"])

	catalog := snapshot.Catalog()
	assert.Equal(t, []string{"drive"}, catalog.Names())
	assert.Zero(t, counting.opens["services/drive.json"], "Names must not read shard bodies")

	_, ok := catalog.Service("drive")
	require.True(t, ok)
	require.Len(t, catalog.Services(), 1)
	assert.Equal(t, 1, counting.opens["services/drive.json"], "one Catalog reads a shard at most once")

	_, ok = snapshot.Catalog().Service("drive")
	require.True(t, ok)
	assert.Equal(t, 2, counting.opens["services/drive.json"], "Snapshot itself must not cache service bodies")
}

func TestEmbeddedSnapshot(t *testing.T) {
	snapshot, err := OpenSnapshot()
	require.NoError(t, err)
	names := snapshot.ServiceNames()
	assert.Equal(t, expectedSnapshotServices, names)
	names[0] = "modified"
	assert.Equal(t, expectedSnapshotServices, snapshot.ServiceNames())

	catalog := snapshot.Catalog()
	assert.Equal(t, expectedSnapshotServices, catalog.Names())
	require.NoError(t, catalog.Preload(expectedSnapshotServices...))
	require.Len(t, catalog.Services(), len(expectedSnapshotServices))
	for i, service := range catalog.Services() {
		assert.Equal(t, expectedSnapshotServices[i], service.Name)
	}
	require.NoError(t, catalog.Err())
}

func TestCatalogIntegrityCauseIsPreservedButRedacted(t *testing.T) {
	const secret = `/private/build/registry https://registry.` +
		`corp.example token=` +
		`super-secret {"name":"drive","secret":"body"}`
	cause := errors.New(secret)
	base := validSnapshotMapFS(t, "drive")
	snapshot, err := OpenSnapshotFS(&failingFS{FS: base, target: "services/drive.json", err: cause})
	require.NoError(t, err)

	err = snapshot.Catalog().Preload("drive")
	requireCatalogIntegrityError(t, err, `embedded catalog service "drive" failed integrity validation: service file is missing`)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, cause, errors.Unwrap(err))

	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.NotContains(t, problem.Message, secret)
	assert.NotContains(t, problem.Hint, secret)

	wire, marshalErr := json.Marshal(err)
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(wire), secret)

	var stderr bytes.Buffer
	require.True(t, output.WriteTypedErrorEnvelope(&stderr, err, ""))
	assert.NotContains(t, stderr.String(), secret)

	var defaultLog bytes.Buffer
	log.New(&defaultLog, "", 0).Print(err)
	assert.NotContains(t, defaultLog.String(), secret)
}

func requireCatalogIntegrityError(t *testing.T, err error, wantMessage string) {
	t.Helper()
	require.Error(t, err)
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.Equal(t, errs.CategoryInternal, problem.Category)
	assert.Equal(t, errs.SubtypeCatalogIntegrity, problem.Subtype)
	assert.Equal(t, wantMessage, problem.Message)
	assert.Equal(t, output.ExitInternal, output.ExitCodeOf(err))
	assert.False(t, problem.Retryable)
	assert.NotNil(t, errors.Unwrap(err))
}

func validSnapshotMapFS(t *testing.T, name string) fstest.MapFS {
	t.Helper()
	body := []byte(`{"name":"` + name + `","version":"v1","servicePath":"/open-apis/` + name + `","resources":{}}`)
	entry := manifestEntryFor(t, name, body)
	services := []any{entry}
	manifest := map[string]any{
		"schema_version": 1,
		"services":       services,
	}
	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)
	return fstest.MapFS{
		"manifest.json":              &fstest.MapFile{Data: manifestBytes},
		"services/" + name + ".json": &fstest.MapFile{Data: body},
	}
}

func manifestEntryFor(t *testing.T, name string, body []byte) map[string]any {
	t.Helper()
	sum := sha256.Sum256(body)
	return map[string]any{
		"name":     name,
		"file":     "services/" + name + ".json",
		"revision": 1,
		"size":     len(body),
		"sha256":   hex.EncodeToString(sum[:]),
	}
}

func rewriteEntryForBody(t *testing.T, fsys fstest.MapFS, body []byte) {
	t.Helper()
	rewriteEntry(t, fsys, func(entry map[string]any) {
		sum := sha256.Sum256(body)
		entry["size"] = len(body)
		entry["sha256"] = hex.EncodeToString(sum[:])
	})
}

func rewriteEntry(t *testing.T, fsys fstest.MapFS, mutate func(map[string]any)) {
	t.Helper()
	rewriteManifest(t, fsys, func(manifest map[string]any) {
		entry := manifest["services"].([]any)[0].(map[string]any)
		mutate(entry)
	})
}

func rewriteManifest(t *testing.T, fsys fstest.MapFS, mutate func(map[string]any)) {
	t.Helper()
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(fsys["manifest.json"].Data, &manifest))
	mutate(manifest)
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	fsys["manifest.json"] = &fstest.MapFile{Data: data}
}

func cloneMapFS(in fstest.MapFS) fstest.MapFS {
	out := make(fstest.MapFS, len(in))
	for name, file := range in {
		clone := *file
		clone.Data = slices.Clone(file.Data)
		out[name] = &clone
	}
	return out
}

type countingFS struct {
	fs.FS
	opens map[string]int
}

func (f *countingFS) Open(name string) (fs.File, error) {
	f.opens[name]++
	return f.FS.Open(name)
}

type failingFS struct {
	fs.FS
	target string
	err    error
}

func (f *failingFS) Open(name string) (fs.File, error) {
	if name == f.target {
		return nil, f.err
	}
	return f.FS.Open(name)
}

func (f *failingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(f.FS, name)
}

type failingSubFS struct {
	fs.FS
	err error
}

func (f *failingSubFS) Sub(string) (fs.FS, error) {
	return nil, f.err
}
