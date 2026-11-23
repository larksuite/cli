// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDocResourcePaths(t *testing.T) {
	for _, kind := range []string{"img", "source", "whiteboard", "html5-block"} {
		t.Run(kind, func(t *testing.T) {
			fixture := "resource contents"
			name := "asset.txt"
			switch kind {
			case "img":
				name, fixture = "asset.png", localDocResourcePNG(t, 3, 2)
			case "whiteboard":
				name, fixture = "asset.svg", `<svg xmlns="http://www.w3.org/2000/svg"/>`
			case "html5-block":
				name, fixture = "asset.html", "<html><body>resource</body></html>"
			}
			runtime := newLocalDocResourceTestRuntime(t, nil)
			if err := os.Mkdir("draft", 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("draft", name), []byte(fixture), 0o600); err != nil {
				t.Fatal(err)
			}
			runtime.Cmd.Annotations = map[string]string{docsContentPathAnnotation: "draft/document.xml"}
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"@./" + name, "@./draft/" + name, "@" + filepath.Join(cwd, "draft", name)} {
				got, err := readDocResourceFixture(runtime, kind, path)
				if err != nil || got != fixture {
					t.Fatalf("read %q = %q, %v; want fixture", path, got, err)
				}
			}
			// A CWD file wins even when the XML directory has a valid namesake.
			cwdFixture := fixture + "\n"
			if err := os.WriteFile(name, []byte(cwdFixture), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := readDocResourceFixture(runtime, kind, "@./"+name)
			if err != nil || got != cwdFixture {
				t.Fatalf("CWD precedence = %q, %v", got, err)
			}
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			// Absolute paths, inline content and stdin have no directory fallback.
			if _, err := readDocResourceFixture(runtime, kind, "@"+filepath.Join(cwd, name)); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("missing absolute path error = %v", err)
			}
			delete(runtime.Cmd.Annotations, docsContentPathAnnotation)
			if _, err := readDocResourceFixture(runtime, kind, "@./"+name); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("missing source path error = %v", err)
			}
			// An allowed absolute path outside CWD also uses the same FileIO policy.
			if goruntime.GOOS == "windows" {
				return // The Unix /tmp allow root has no Windows equivalent.
			}
			outside, err := os.MkdirTemp("/tmp", "lark-doc-resource-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(outside) })
			abs := filepath.Join(outside, name)
			if err := os.WriteFile(abs, []byte(fixture), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err = readDocResourceFixture(runtime, kind, "@"+abs)
			if err != nil || got != fixture {
				t.Fatalf("outside absolute path = %q, %v", got, err)
			}
		})
	}
}

func readDocResourceFixture(runtime *common.RuntimeContext, kind, path string) (string, error) {
	switch kind {
	case "html5-block":
		return readHTML5BlockPath(runtime, path, "html5-block path")
	case "whiteboard":
		return readWhiteboardPath(runtime, path, "svg")
	default:
		resourceKind := localDocResourceFile
		if kind == "img" {
			resourceKind = localDocResourceImage
		}
		resource, err := newLocalDocResource(runtime, resourceKind, path, 1)
		if err != nil {
			return "", err
		}
		data, err := cmdutil.ReadInputFile(runtime.FileIO(), resource.Path)
		return string(data), err
	}
}

type deniedDocResourceFileIO struct {
	fileio.FileIO
	err   error
	paths []string
}

func (f *deniedDocResourceFileIO) Stat(path string) (fileio.FileInfo, error) {
	f.paths = append(f.paths, path)
	return nil, f.err
}

func TestDocResourcePathDoesNotFallBackOnAccessFailure(t *testing.T) {
	for _, failure := range []error{fs.ErrPermission, &fileio.PathValidationError{Err: fs.ErrNotExist}} {
		f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("resource-path-denied"))
		fio := &deniedDocResourceFileIO{FileIO: f.ResolveFileIO(context.Background()), err: failure}
		f.FileIOProvider = docsScriptFileIOProvider{fileIO: fio}
		runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{
			Use: "test", Annotations: map[string]string{docsContentPathAnnotation: "draft/document.xml"},
		}, docsTestConfigWithAppID("resource-path-denied"), f, core.AsUser)
		_, _, err := statDocResource(runtime, "asset.html")
		if !errors.Is(err, failure) || len(fio.paths) != 1 || fio.paths[0] != "asset.html" {
			t.Fatalf("error = %v, attempted paths = %v", err, fio.paths)
		}
	}
}
