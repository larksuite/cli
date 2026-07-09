// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs" //nolint:depguard // git credential metadata is CLI config-dir state, not user file I/O.
)

type AppStorage interface {
	Read(appID, key string) ([]byte, error)
	Write(appID, key string, data []byte) error
	Delete(appID, key string) error
}

type Store struct {
	path    string
	appID   string
	storage AppStorage
}

func NewStore() *Store {
	return &Store{path: filepath.Join(core.GetConfigDir(), MetadataFilename)}
}

func NewAppStore(appID string, storage AppStorage) *Store {
	return &Store{appID: appID, storage: storage}
}

func NewStoreAt(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	if s.storage != nil {
		return fmt.Sprintf("apps:%s/%s", s.appID, MetadataFilename)
	}
	return s.path
}

func (s *Store) Load() (*CredentialFile, error) {
	data, err := s.read()
	if errors.Is(err, fs.ErrNotExist) {
		return &CredentialFile{Version: CurrentCredentialVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &CredentialFile{Version: CurrentCredentialVersion}, nil
	}
	var file CredentialFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig, "invalid %s: %s", MetadataFilename, err).
			WithHint("the local Git credential metadata is damaged; rerun `lark-cli apps +git-credential-init --app-id <app_id>` after backing up or removing the damaged app metadata").
			WithCause(err)
	}
	if file.Version == 0 {
		file.Version = CurrentCredentialVersion
	}
	if file.Version > CurrentCredentialVersion {
		return nil, &errs.ConfigError{Problem: errs.Problem{
			Category: errs.CategoryConfig,
			Subtype:  errs.SubtypeInvalidConfig,
			Message:  fmt.Sprintf("%s version %d is newer than supported version %d", MetadataFilename, file.Version, CurrentCredentialVersion),
			Hint:     "upgrade lark-cli and retry",
		}}
	}
	return &file, nil
}

func (s *Store) Save(file *CredentialFile) error {
	if file == nil {
		file = &CredentialFile{}
	}
	file.Version = CurrentCredentialVersion
	data, _ := json.MarshalIndent(file, "", "  ")
	return s.write(append(data, '\n'))
}

func (s *Store) Upsert(record CredentialRecord) error {
	file, err := s.Load()
	if err != nil {
		return err
	}
	file.CredentialRecord = record
	return s.Save(file)
}

func (s *Store) Current() (*CredentialRecord, error) {
	file, err := s.Load()
	if err != nil {
		return nil, err
	}
	if file.GitHTTPURL == "" {
		return nil, nil
	}
	return &file.CredentialRecord, nil
}

func (s *Store) DeleteByURL(gitHTTPURL string) (*CredentialRecord, error) {
	file, err := s.Load()
	if err != nil {
		return nil, err
	}
	if file.GitHTTPURL != gitHTTPURL || file.GitHTTPURL == "" {
		return nil, nil
	}
	record := file.CredentialRecord
	if err := s.delete(); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Store) FindByURL(gitHTTPURL string) (*CredentialRecord, error) {
	file, err := s.Load()
	if err != nil {
		return nil, err
	}
	if file.GitHTTPURL != gitHTTPURL || file.GitHTTPURL == "" {
		return nil, nil
	}
	return &file.CredentialRecord, nil
}

func (s *Store) Records() ([]CredentialRecord, error) {
	file, err := s.Load()
	if err != nil {
		return nil, err
	}
	if file.GitHTTPURL == "" {
		return []CredentialRecord{}, nil
	}
	return []CredentialRecord{file.CredentialRecord}, nil
}

func (s *Store) FindByAppID(appID string, profile ProfileContext) ([]CredentialRecord, error) {
	records, err := s.Records()
	if err != nil {
		return nil, err
	}
	out := make([]CredentialRecord, 0)
	for _, record := range records {
		if record.AppID != appID {
			continue
		}
		if profile.Profile != "" && record.Profile != profile.Profile {
			continue
		}
		if profile.ProfileAppID != "" && record.ProfileAppID != profile.ProfileAppID {
			continue
		}
		if profile.UserOpenID != "" && record.UserOpenID != profile.UserOpenID {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *Store) read() ([]byte, error) {
	if s.storage != nil {
		data, err := s.storage.Read(s.appID, MetadataFilename)
		if data == nil && err == nil {
			return nil, fs.ErrNotExist
		}
		return data, err
	}
	return vfs.ReadFile(s.path)
}

func (s *Store) write(data []byte) error {
	if s.storage != nil {
		return s.storage.Write(s.appID, MetadataFilename, data)
	}
	if err := vfs.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	return validate.AtomicWrite(s.path, data, 0600)
}

func (s *Store) delete() error {
	if s.storage != nil {
		return s.storage.Delete(s.appID, MetadataFilename)
	}
	if err := vfs.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
