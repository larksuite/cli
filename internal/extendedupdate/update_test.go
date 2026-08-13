// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package extendedupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{"linux", "amd64", "lark-cli-extended-1.2.3-linux-amd64.tar.gz"},
		{"darwin", "arm64", "lark-cli-extended-1.2.3-darwin-arm64.tar.gz"},
		{"windows", "amd64", "lark-cli-extended-1.2.3-windows-amd64.zip"},
	}
	for _, tt := range tests {
		got, err := assetName("1.2.3", tt.goos, tt.goarch)
		if err != nil || got != tt.want {
			t.Fatalf("assetName(%q, %q) = %q, %v; want %q", tt.goos, tt.goarch, got, err, tt.want)
		}
	}
}

func TestChecksumForSelectsExactAsset(t *testing.T) {
	want := sha256.Sum256([]byte("extended"))
	data := []byte(fmt.Sprintf("%x  lark-cli-1.2.3-linux-amd64.tar.gz\n%x  lark-cli-extended-1.2.3-linux-amd64.tar.gz\n",
		sha256.Sum256([]byte("standard")), want))
	got, err := checksumFor(data, "lark-cli-extended-1.2.3-linux-amd64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want[:]) {
		t.Fatalf("checksum = %x, want %x", got, want)
	}
}

func TestExtractBinary(t *testing.T) {
	const payload = "extended-binary"
	var tgz bytes.Buffer
	gz := gzip.NewWriter(&tgz)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "lark-cli", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := extractBinary(tgz.Bytes(), "linux")
	if err != nil || string(got) != payload {
		t.Fatalf("tar extract = %q, %v", got, err)
	}

	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	file, err := zw.Create("lark-cli.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err = extractBinary(zipped.Bytes(), "windows")
	if err != nil || string(got) != payload {
		t.Fatalf("zip extract = %q, %v", got, err)
	}
}
