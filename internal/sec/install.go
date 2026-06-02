// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/larksuite/cli/internal/client"
)

// Installer orchestrates first-time install of lark-sec-cli:
// fetch remote manifest via OAPI → download zip → extract into
// versions/<version>/ → swap "current" → write state.json.
//
// After this first install, lark-sec-cli takes over its own updates and
// lark-cli is no longer in the update path. The installer therefore only
// knows about the bootstrap path — no Tron, no other release sources.
type Installer struct {
	Paths      *Paths
	HTTPClient *http.Client
	// APIClientFunc resolves the OAPI client lazily. It is invoked only when
	// the install pipeline actually needs to fetch the remote manifest —
	// short-circuits (and other callers of installer() that don't install,
	// like sec status / sec stop) avoid keychain decryption entirely.
	APIClientFunc func() (*client.APIClient, error)
}

// InstallOptions tunes a single Install call.
type InstallOptions struct {
	// Force re-runs the pipeline even when an install already exists. Used by
	// `sec install --force` for repair / re-pinning to the bundled bootstrap.
	Force bool
	// Region selects which region's URLs to pick from the manifest. Defaults to
	// DefaultRegion ("cn"). Reserved for future brand split.
	Region string
	// Verbose, when non-nil, is the destination for step-by-step trace output.
	// nil = silent (production default); typically set to stderr by `sec install -v`.
	Verbose io.Writer
}

// tracef writes one trace line to w if w is non-nil.
func tracef(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "[sec install] "+format+"\n", args...)
}

// Install runs the bootstrap pipeline and returns the new State on success.
// If a usable install already exists on disk and Force is false, returns the
// existing state unchanged (no network call).
func (i *Installer) Install(ctx context.Context, opts InstallOptions) (*State, error) {
	v := opts.Verbose
	tracef(v, "ensuring sec paths under %s", i.Paths.InstallDir())
	if err := i.Paths.Ensure(); err != nil {
		return nil, err
	}

	tracef(v, "loading existing state from %s", i.Paths.StateFile())
	existing, err := LoadState(i.Paths.StateFile())
	if err != nil {
		return nil, fmt.Errorf("load sec state: %w", err)
	}
	if existing != nil {
		tracef(v, "existing state: version=%s binary=%s", existing.Version, existing.BinaryPath)
	} else {
		tracef(v, "no existing state on disk")
	}

	// Idempotent short-circuit: nothing to do if an install is already on disk.
	// Self-upgrades after bootstrap are lark-sec-cli's job, not ours — see the
	// upgrade subsystem in lark-sec-cli/internal/upgrade/.
	if !opts.Force && existing != nil && binaryReady(existing.BinaryPath) {
		tracef(v, "binary exists at %s — short-circuiting (no network)", existing.BinaryPath)
		return existing, nil
	}
	if opts.Force {
		tracef(v, "--force set; running full install pipeline")
	} else {
		tracef(v, "no usable install on disk; running full install pipeline")
	}

	region := opts.Region
	if region == "" {
		region = DefaultRegion
	}
	tracef(v, "region=%s", region)

	if i.APIClientFunc == nil {
		return nil, errors.New("sec installer: APIClientFunc is required to fetch remote manifest")
	}
	tracef(v, "resolving OAPI client (will decrypt credentials)")
	apiClient, err := i.APIClientFunc()
	if err != nil {
		return nil, fmt.Errorf("resolve api client: %w", err)
	}
	platform, arch, err := CurrentPlatformArch()
	if err != nil {
		return nil, err
	}
	tracef(v, "detected platform=%s arch=%s", platform, arch)

	tracef(v, "fetching remote manifest from %s", secCliManifestPath)
	rm, err := FetchRemoteManifest(ctx, apiClient, region, platform, arch, v)
	if err != nil {
		return nil, err
	}
	tracef(v, "manifest returned %d url(s): %v", len(rm.URLs), rm.URLs)
	downloadURL := rm.URLs[0]
	tracef(v, "picked downloadURL=%s", downloadURL)
	version, err := versionFromURL(downloadURL)
	if err != nil {
		return nil, err
	}
	tracef(v, "parsed version=%s", version)

	versionDir := i.Paths.VersionDir(version)
	tracef(v, "creating versionDir=%s", versionDir)
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return nil, err
	}
	zipPath := filepath.Join(i.Paths.VersionsDir(), version+".zip")

	tracef(v, "downloading %s -> %s", downloadURL, zipPath)
	if err := Download(ctx, DownloadOptions{
		URL:         downloadURL,
		Destination: zipPath,
		HTTPClient:  i.HTTPClient,
	}); err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(zipPath); statErr == nil {
		tracef(v, "downloaded %d bytes", info.Size())
	}
	defer os.Remove(zipPath) // free disk; we keep the unpacked version dir

	tracef(v, "extracting %s -> %s", zipPath, versionDir)
	if err := ExtractZip(zipPath, versionDir); err != nil {
		return nil, err
	}

	binaryPath, err := locateBinary(versionDir)
	if err != nil {
		return nil, err
	}
	tracef(v, "located binary at %s", binaryPath)
	// Ensure executable bit on POSIX — some zips lose it.
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(binaryPath); err == nil {
			_ = os.Chmod(binaryPath, info.Mode()|0o100|0o010|0o001)
		}
	}

	tracef(v, "swapping %s -> %s", i.Paths.CurrentLink(), versionDir)
	if err := swapCurrent(i.Paths.CurrentLink(), versionDir); err != nil {
		return nil, fmt.Errorf("swap current: %w", err)
	}

	tracef(v, "writing state.json to %s", i.Paths.StateFile())
	state := &State{
		Version:     version,
		InstalledAt: time.Now().UTC(),
		BinaryPath:  i.Paths.BinaryPath(),
	}
	if err := SaveState(i.Paths.StateFile(), state); err != nil {
		return nil, err
	}
	return state, nil
}

// locateBinary handles two artifact layouts: flat (zip root has the binary)
// and nested (zip root is a single dir containing the binary). The bootstrap
// manifest's example payload uses nested ("linux-amd64-1.0.1-alpha.23/...");
// we accommodate either since the wrapping dir name could change per build.
func locateBinary(versionDir string) (string, error) {
	name := BinaryName()

	flat := filepath.Join(versionDir, name)
	if _, err := os.Stat(flat); err == nil {
		return flat, nil
	}

	var found string
	walkErr := filepath.WalkDir(versionDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if found == "" {
		return "", fmt.Errorf("binary %q not found under %s", name, versionDir)
	}

	// Promote the binary's parent to be versionDir so "current" → versionDir
	// produces a predictable layout. Move the *contents* up rather than the
	// binary alone, because shared libs may sit beside it.
	parent := filepath.Dir(found)
	if parent != versionDir {
		entries, err := os.ReadDir(parent)
		if err != nil {
			return "", err
		}
		for _, e := range entries {
			if err := os.Rename(filepath.Join(parent, e.Name()), filepath.Join(versionDir, e.Name())); err != nil {
				return "", err
			}
		}
		_ = os.Remove(parent)
	}
	return filepath.Join(versionDir, name), nil
}

// swapCurrent atomically points <install>/current at versionDir. On POSIX
// we use a symlink with the standard rename-into-place trick; on Windows we
// fall back to removing the directory and copying, since junctions need
// admin / developer-mode privileges we may not have.
func swapCurrent(link, versionDir string) error {
	if runtime.GOOS == "windows" {
		// Remove any existing target then copy. This is non-atomic, but
		// concurrent installs on the same Windows host are not a use case
		// we support — `sec install` runs interactively.
		if err := os.RemoveAll(link); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return copyDir(versionDir, link)
	}

	tmp := link + ".new"
	_ = os.Remove(tmp)
	if err := os.Symlink(versionDir, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func binaryReady(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
