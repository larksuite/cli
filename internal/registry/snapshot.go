// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"sort"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

const catalogSchemaVersion = 1

var (
	serviceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

//go:embed catalog/manifest.json catalog/services/*.json
var embeddedCatalogFS embed.FS

// Manifest describes the immutable service shards in a catalog snapshot.
type Manifest struct {
	SchemaVersion int                    `json:"schema_version"`
	Services      []ManifestServiceEntry `json:"services"`
}

// ManifestServiceEntry records the identity and digest of one service shard.
type ManifestServiceEntry struct {
	Name     string `json:"name"`
	File     string `json:"file"`
	Revision int    `json:"revision"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}

// Snapshot is an immutable manifest paired with its backing filesystem. It is
// the apicatalog.Loader behind the embedded catalog: opening it reads only the
// manifest, and a service body is validated and parsed the first time a
// Catalog navigates into it.
type Snapshot struct {
	manifest Manifest
	fs       fs.FS
}

// OpenSnapshot opens the catalog embedded in the binary.
func OpenSnapshot() (*Snapshot, error) {
	return openSnapshot(embeddedCatalogFS, "catalog")
}

func openSnapshot(fsys fs.FS, root string) (*Snapshot, error) {
	fsys, err := fs.Sub(fsys, root)
	if err != nil {
		return nil, catalogAccessError("embedded catalog filesystem could not be opened", err)
	}
	return OpenSnapshotFS(fsys)
}

// OpenSnapshotFS validates a snapshot manifest and its exact service file set.
// It intentionally does not read any service file body.
func OpenSnapshotFS(fsys fs.FS) (*Snapshot, error) {
	data, err := fs.ReadFile(fsys, "manifest.json")
	if err != nil {
		return nil, catalogAccessError("embedded catalog manifest could not be read", err)
	}

	manifest, err := parseManifest(data)
	if err != nil {
		return nil, err
	}
	if err := validateServiceFileSet(fsys, manifest.Services); err != nil {
		return nil, err
	}
	return &Snapshot{manifest: manifest, fs: fsys}, nil
}

// ServiceNames returns the manifest's sorted service names as a new slice.
func (s *Snapshot) ServiceNames() []string {
	names := make([]string, len(s.manifest.Services))
	for i, entry := range s.manifest.Services {
		names[i] = entry.Name
	}
	return names
}

// Names implements apicatalog.Loader from the manifest alone.
func (s *Snapshot) Names() []string { return s.ServiceNames() }

// Load implements apicatalog.Loader: it validates one service shard against
// its manifest entry and parses it. Failures are typed catalog-integrity
// errors; a name absent from the manifest is apicatalog.ErrServiceNotFound.
func (s *Snapshot) Load(name string) (meta.Service, error) {
	entry, ok := s.manifestEntry(name)
	if !ok {
		return meta.Service{}, apicatalog.ErrServiceNotFound
	}

	data, err := fs.ReadFile(s.fs, entry.File)
	if err != nil {
		return meta.Service{}, serviceIntegrityError(name, "service file is missing", err)
	}
	if int64(len(data)) != entry.Size {
		cause := catalogIntegrityCause("service file size does not match manifest")
		return meta.Service{}, serviceIntegrityError(name, "size mismatch", cause)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != entry.SHA256 {
		cause := catalogIntegrityCause("service file sha256 does not match manifest")
		return meta.Service{}, serviceIntegrityError(name, "sha256 mismatch", cause)
	}

	var service meta.Service
	if err := decodeOne(data, &service, false); err != nil {
		return meta.Service{}, serviceIntegrityError(name, "invalid JSON", err)
	}
	if service.Name != name {
		cause := catalogIntegrityCause("service body name does not match manifest")
		return meta.Service{}, serviceIntegrityError(name, "service name mismatch", cause)
	}
	return service, nil
}

// Catalog returns a lazy navigation handle over every service in the
// snapshot. No shard is read until it is navigated; callers that must fail
// early for a corrupt shard use Catalog.Preload.
func (s *Snapshot) Catalog() apicatalog.Catalog {
	return apicatalog.NewLazy(apicatalog.SourceEmbedded, s)
}

func parseManifest(data []byte) (Manifest, error) {
	var fields map[string]json.RawMessage
	if err := decodeOne(data, &fields, false); err != nil {
		return Manifest{}, manifestIntegrityError("invalid JSON", err)
	}

	var manifest Manifest
	if raw, ok := fields["schema_version"]; ok {
		if err := decodeOne(raw, &manifest.SchemaVersion, false); err != nil {
			return Manifest{}, manifestIntegrityError("invalid JSON", err)
		}
	}
	if manifest.SchemaVersion != catalogSchemaVersion {
		cause := catalogIntegrityCause("unsupported catalog schema version")
		return Manifest{}, manifestIntegrityError(
			fmt.Sprintf("unsupported schema_version %d", manifest.SchemaVersion),
			cause,
		)
	}
	rawServices, ok := fields["services"]
	if !ok {
		cause := catalogIntegrityCause("services field is missing")
		return Manifest{}, manifestIntegrityError("services must be a non-empty sorted array", cause)
	}
	var entries []json.RawMessage
	if err := decodeOne(rawServices, &entries, false); err != nil || len(entries) == 0 {
		if err == nil {
			err = catalogIntegrityCause("services array is empty")
		}
		return Manifest{}, manifestIntegrityError("services must be a non-empty sorted array", err)
	}

	manifest.Services = make([]ManifestServiceEntry, 0, len(entries))
	for _, raw := range entries {
		var entry ManifestServiceEntry
		if err := decodeOne(raw, &entry, true); err != nil {
			return Manifest{}, manifestIntegrityError("invalid or duplicate service entry", err)
		}
		if !validManifestEntry(entry) {
			cause := catalogIntegrityCause("service entry field is invalid")
			return Manifest{}, manifestIntegrityError("invalid or duplicate service entry", cause)
		}
		if len(manifest.Services) > 0 {
			previous := manifest.Services[len(manifest.Services)-1].Name
			switch {
			case previous == entry.Name:
				cause := catalogIntegrityCause("service entry name is duplicated")
				return Manifest{}, manifestIntegrityError("invalid or duplicate service entry", cause)
			case previous > entry.Name:
				cause := catalogIntegrityCause("services array is not sorted")
				return Manifest{}, manifestIntegrityError("services must be a non-empty sorted array", cause)
			}
		}
		manifest.Services = append(manifest.Services, entry)
	}
	return manifest, nil
}

func validManifestEntry(entry ManifestServiceEntry) bool {
	return serviceNamePattern.MatchString(entry.Name) &&
		entry.File == "services/"+entry.Name+".json" &&
		entry.Revision >= 1 &&
		entry.Size > 0 &&
		sha256Pattern.MatchString(entry.SHA256)
}

func validateServiceFileSet(fsys fs.FS, entries []ManifestServiceEntry) error {
	dirEntries, err := fs.ReadDir(fsys, "services")
	if err != nil {
		return manifestIntegrityError("service file set does not match manifest", err)
	}

	expected := make([]string, len(entries))
	for i, entry := range entries {
		expected[i] = entry.File
	}
	sort.Strings(expected)
	actual := make([]string, 0, len(dirEntries))
	for _, entry := range dirEntries {
		if entry.IsDir() {
			cause := catalogIntegrityCause("services directory contains a nested directory")
			return manifestIntegrityError("service file set does not match manifest", cause)
		}
		actual = append(actual, "services/"+entry.Name())
	}
	sort.Strings(actual)
	if !equalStrings(actual, expected) {
		cause := catalogIntegrityCause("services directory entries do not match manifest")
		return manifestIntegrityError("service file set does not match manifest", cause)
	}
	return nil
}

func decodeOne(data []byte, target any, disallowUnknown bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if disallowUnknown {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return catalogIntegrityCause("JSON contains multiple values")
		}
		return err
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *Snapshot) manifestEntry(name string) (ManifestServiceEntry, bool) {
	index := sort.Search(len(s.manifest.Services), func(i int) bool {
		return s.manifest.Services[i].Name >= name
	})
	if index == len(s.manifest.Services) || s.manifest.Services[index].Name != name {
		return ManifestServiceEntry{}, false
	}
	return s.manifest.Services[index], true
}

func manifestIntegrityError(reason string, cause error) error {
	return errs.NewInternalError(
		errs.SubtypeCatalogIntegrity,
		"embedded catalog manifest is invalid: %s",
		reason,
	).WithCause(cause)
}

func catalogAccessError(message string, cause error) error {
	return errs.NewInternalError(
		errs.SubtypeCatalogIntegrity,
		message,
	).WithHint("run lark-cli update to restore the embedded catalog").WithCause(cause)
}

func serviceIntegrityError(name, reason string, cause error) error {
	return errs.NewInternalError(
		errs.SubtypeCatalogIntegrity,
		`embedded catalog service "%s" failed integrity validation: %s`,
		safeServiceName(name),
		reason,
	).WithCause(cause)
}

func safeServiceName(name string) string {
	if serviceNamePattern.MatchString(name) {
		return name
	}
	return "unknown"
}

type catalogIntegrityCause string

func (e catalogIntegrityCause) Error() string { return string(e) }
