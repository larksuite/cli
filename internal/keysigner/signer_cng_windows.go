//go:build windows

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cngSoftwareProvider = "Microsoft Software Key Storage Provider"
	cngPlatformProvider = "Microsoft Platform Crypto Provider"
	cngKeyUsageProperty = "Key Usage"
	cngExportPolicy     = "Export Policy"

	cngAllowSigning = 0x00000002
	cngPersistFlag  = 0x80000000
	cngSilentFlag   = 0x00000040

	nteExists    = 0x8009000F
	nteNotFound  = 0x80090011
	nteBadKeyset = 0x80090016
)

var (
	ncryptDLL                 = windows.NewLazySystemDLL("ncrypt.dll")
	ncryptOpenStorageProvider = ncryptDLL.NewProc("NCryptOpenStorageProvider")
	ncryptCreatePersistedKey  = ncryptDLL.NewProc("NCryptCreatePersistedKey")
	ncryptOpenKey             = ncryptDLL.NewProc("NCryptOpenKey")
	ncryptSetProperty         = ncryptDLL.NewProc("NCryptSetProperty")
	ncryptGetProperty         = ncryptDLL.NewProc("NCryptGetProperty")
	ncryptFinalizeKey         = ncryptDLL.NewProc("NCryptFinalizeKey")
	ncryptExportKey           = ncryptDLL.NewProc("NCryptExportKey")
	ncryptSignHash            = ncryptDLL.NewProc("NCryptSignHash")
	ncryptDeleteKey           = ncryptDLL.NewProc("NCryptDeleteKey")
	ncryptFreeObject          = ncryptDLL.NewProc("NCryptFreeObject")
)

type cngSigner struct {
	software bool
}

// A software algorithm is usable here only when it also implements CNG's
// native key format and signing operations.
type cngAlgorithm interface {
	name() string
	cngKeyParameters() (string, uint32)
	cngPublicKey(windows.Handle) (crypto.PublicKey, error)
	signCNG(windows.Handle, []byte) ([]byte, error)
}

const cngECCPublicBlob = "ECCPUBLICBLOB"

var _ cngAlgorithm = es256Algorithm{}

func (es256Algorithm) cngKeyParameters() (string, uint32) { return "ECDSA_P256", 0 }

func (es256Algorithm) cngPublicKey(key windows.Handle) (crypto.PublicKey, error) {
	blob, err := exportCNGPublicKey(key, cngECCPublicBlob)
	if err != nil {
		return nil, err
	}
	if len(blob) != 72 || binary.LittleEndian.Uint32(blob[:4]) != 0x31534345 {
		return nil, fmt.Errorf("keysigner: invalid CNG P-256 public blob length %d", len(blob))
	}
	coordinateBytes := int(binary.LittleEndian.Uint32(blob[4:8]))
	if coordinateBytes != 32 {
		return nil, fmt.Errorf("keysigner: invalid CNG P-256 coordinate length %d", coordinateBytes)
	}
	public := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(blob[8:40]),
		Y:     new(big.Int).SetBytes(blob[40:72]),
	}
	return P256PublicKey(public)
}

func (a es256Algorithm) signCNG(key windows.Handle, signingInput []byte) ([]byte, error) {
	digest := a.digest(signingInput)
	var size uint32
	status, _, _ := ncryptSignHash.Call(
		uintptr(key),
		0,
		uintptr(unsafe.Pointer(&digest[0])),
		uintptr(len(digest)),
		0,
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if status != 0 {
		return nil, cngError("measure ECDSA signature", status)
	}
	signature := make([]byte, size)
	status, _, _ = ncryptSignHash.Call(
		uintptr(key),
		0,
		uintptr(unsafe.Pointer(&digest[0])),
		uintptr(len(digest)),
		uintptr(unsafe.Pointer(&signature[0])),
		uintptr(len(signature)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if status != 0 {
		return nil, cngError("sign digest", status)
	}
	signature = signature[:size]
	if len(signature) != 64 {
		return nil, fmt.Errorf("keysigner: CNG returned %d-byte P-256 signature", len(signature))
	}
	return signature, nil
}

var _ cngAlgorithm = rs256Algorithm{}

func (rs256Algorithm) cngKeyParameters() (string, uint32) { return "RSA", 2048 }

func (a rs256Algorithm) cngPublicKey(key windows.Handle) (crypto.PublicKey, error) {
	blob, err := exportCNGPublicKey(key, "RSAPUBLICBLOB")
	if err != nil {
		return nil, err
	}
	// BCRYPT_RSAKEY_BLOB has six little-endian ULONG fields, followed by the
	// big-endian exponent and modulus. Reject private blobs and malformed sizes.
	if len(blob) < 24 || binary.LittleEndian.Uint32(blob[:4]) != 0x31415352 {
		return nil, ErrCorrupt
	}
	bits := binary.LittleEndian.Uint32(blob[4:8])
	exponentBytes := binary.LittleEndian.Uint32(blob[8:12])
	modulusBytes := binary.LittleEndian.Uint32(blob[12:16])
	if bits < 2048 || bits > 16384 || bits%8 != 0 || modulusBytes != bits/8 ||
		exponentBytes == 0 || exponentBytes > 4 ||
		binary.LittleEndian.Uint32(blob[16:20]) != 0 || binary.LittleEndian.Uint32(blob[20:24]) != 0 ||
		uint64(len(blob)) != 24+uint64(exponentBytes)+uint64(modulusBytes) {
		return nil, ErrCorrupt
	}
	exponent := new(big.Int).SetBytes(blob[24 : 24+exponentBytes]).Uint64()
	if exponent > 1<<31-1 {
		return nil, ErrCorrupt
	}
	public := &rsa.PublicKey{N: new(big.Int).SetBytes(blob[24+exponentBytes:]), E: int(exponent)}
	if public.N.BitLen() != int(bits) {
		return nil, ErrCorrupt
	}
	if err := a.validatePublicKey(public); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return public, nil
}

func (a rs256Algorithm) signCNG(key windows.Handle, input []byte) ([]byte, error) {
	public, err := a.cngPublicKey(key)
	if err != nil {
		return nil, err
	}
	sha256Name, err := windows.UTF16PtrFromString("SHA256")
	if err != nil {
		return nil, err
	}
	padding := struct{ algorithm *uint16 }{sha256Name}
	digest := sha256.Sum256(input)
	signature := make([]byte, public.(*rsa.PublicKey).Size())
	var size uint32
	const padPKCS1 = 0x00000002
	status, _, _ := ncryptSignHash.Call(uintptr(key), uintptr(unsafe.Pointer(&padding)),
		uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)),
		uintptr(unsafe.Pointer(&signature[0])), uintptr(len(signature)),
		uintptr(unsafe.Pointer(&size)), padPKCS1|cngSilentFlag)
	if status != 0 {
		return nil, cngError("sign RSA digest", status)
	}
	if size != uint32(len(signature)) {
		return nil, fmt.Errorf("%w: CNG returned %d-byte RSA signature", ErrCorrupt, size)
	}
	if err := rsa.VerifyPKCS1v15(public.(*rsa.PublicKey), crypto.SHA256, digest[:], signature); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return signature, nil
}

func (s cngSigner) algorithmForRef(ctx context.Context, ref KeyRef) (cngAlgorithm, error) {
	algorithm, err := algorithmForRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	native, ok := algorithm.(cngAlgorithm)
	if !ok {
		return nil, fmt.Errorf("keysigner: CNG: %w: %q", ErrUnsupportedAlgorithm, ref.Algorithm)
	}
	return native, nil
}

// NewSigner constructs only the named native backend, or returns nil if this
// platform does not support it. Windows does not use the directory resolver.
func NewSigner(name string, _ func(string) (string, error)) Signer {
	switch name {
	case WindowsPlatformKSPSignerName:
		return cngSigner{}
	case WindowsSoftwareKSPSignerName:
		return cngSigner{software: true}
	}
	return nil
}

func (s cngSigner) Name() string {
	if s.software {
		return WindowsSoftwareKSPSignerName
	}
	return WindowsPlatformKSPSignerName
}

func (s cngSigner) SecurityLevel() SecurityLevel {
	if s.software {
		return SecurityLevelL2
	}
	return SecurityLevelL1
}

func (s cngSigner) EnsureKey(ctx context.Context, ref KeyRef) (_ crypto.PublicKey, retErr error) {
	selected, err := s.algorithmForRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	provider, err := s.openProvider()
	if err != nil {
		return nil, err
	}
	defer freeCNGObject(provider)

	keyName, err := windows.UTF16PtrFromString(s.containerName(ref.Label))
	if err != nil {
		return nil, err
	}
	if key, openErr := openCNGKey(provider, keyName); openErr == nil {
		defer freeCNGObject(key)
		return selected.cngPublicKey(key)
	} else if !errors.Is(openErr, ErrKeyNotFound) {
		return nil, openErr
	}

	algorithmName, bits := selected.cngKeyParameters()
	algorithm, err := windows.UTF16PtrFromString(algorithmName)
	if err != nil {
		return nil, err
	}
	var key windows.Handle
	status, _, _ := ncryptCreatePersistedKey.Call(
		uintptr(provider),
		uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(algorithm)),
		uintptr(unsafe.Pointer(keyName)),
		0,
		0,
	)
	if uint32(status) == nteExists {
		opened, openErr := openCNGKey(provider, keyName)
		if openErr != nil {
			return nil, openErr
		}
		defer freeCNGObject(opened)
		return selected.cngPublicKey(opened)
	}
	if status != 0 {
		return nil, cngError("create persisted key", status)
	}
	finalized := false
	defer func() {
		if retErr != nil && finalized {
			// Platform KSP rejects NCRYPT_SILENT_FLAG for deletion.
			status, _, _ := ncryptDeleteKey.Call(uintptr(key), 0)
			if status == 0 {
				return
			}
			retErr = errors.Join(retErr, cngError(fmt.Sprintf("clean up new key %q", ref.Label), status))
		}
		freeCNGObject(key)
	}()

	if bits != 0 {
		if err := setCNGUint32(key, "Length", bits); err != nil {
			return nil, err
		}
	}
	property, err := windows.UTF16PtrFromString(cngKeyUsageProperty)
	if err != nil {
		return nil, err
	}
	usage := uint32(cngAllowSigning)
	status, _, _ = ncryptSetProperty.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(property)),
		uintptr(unsafe.Pointer(&usage)),
		unsafe.Sizeof(usage),
		cngPersistFlag,
	)
	if status != 0 {
		return nil, cngError("restrict key usage to signing", status)
	}
	exportProperty, err := windows.UTF16PtrFromString(cngExportPolicy)
	if err != nil {
		return nil, err
	}
	// PCP only accepts export-policy SET for archiving. Its generated TPM
	// keys are non-exportable by default; verify that policy after finalization.
	if s.software {
		noExport := uint32(0)
		status, _, _ = ncryptSetProperty.Call(
			uintptr(key),
			uintptr(unsafe.Pointer(exportProperty)),
			uintptr(unsafe.Pointer(&noExport)),
			unsafe.Sizeof(noExport),
			cngPersistFlag,
		)
		if status != 0 {
			return nil, cngError("disable private-key export", status)
		}
	}
	status, _, _ = ncryptFinalizeKey.Call(uintptr(key), cngSilentFlag)
	if status != 0 {
		return nil, cngError("finalize key", status)
	}
	finalized = true
	var exportPolicy, propertySize uint32
	status, _, _ = ncryptGetProperty.Call(uintptr(key), uintptr(unsafe.Pointer(exportProperty)),
		uintptr(unsafe.Pointer(&exportPolicy)), unsafe.Sizeof(exportPolicy),
		uintptr(unsafe.Pointer(&propertySize)), 0)
	if status != 0 {
		return nil, cngError("read private-key export policy", status)
	}
	if propertySize != 4 || exportPolicy != 0 {
		return nil, fmt.Errorf("%w: CNG key permits private-key export", ErrCorrupt)
	}
	return selected.cngPublicKey(key)
}

func (s cngSigner) PublicKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error) {
	algorithm, err := s.algorithmForRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	key, cleanup, err := s.loadKey(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return algorithm.cngPublicKey(key)
}

func (s cngSigner) Sign(ctx context.Context, ref KeyRef, signingInput []byte) ([]byte, string, error) {
	algorithm, err := s.algorithmForRef(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	key, cleanup, err := s.loadKey(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()
	signature, err := algorithm.signCNG(key, signingInput)
	if err != nil {
		return nil, "", err
	}
	return signature, algorithm.name(), nil
}

func (s cngSigner) DeleteKey(ctx context.Context, ref KeyRef) error {
	algorithm, err := s.algorithmForRef(ctx, ref)
	if err != nil {
		return err
	}
	provider, err := s.openProvider()
	if err != nil {
		return err
	}
	defer freeCNGObject(provider)
	keyName, err := windows.UTF16PtrFromString(s.containerName(ref.Label))
	if err != nil {
		return err
	}
	key, err := openCNGKey(provider, keyName)
	if err != nil {
		if errors.Is(err, ErrKeyNotFound) {
			return nil
		}
		return err
	}
	if _, err := algorithm.cngPublicKey(key); err != nil {
		freeCNGObject(key)
		return err
	}
	// Platform KSP rejects NCRYPT_SILENT_FLAG for deletion.
	status, _, _ := ncryptDeleteKey.Call(uintptr(key), 0)
	if status != 0 {
		freeCNGObject(key)
		return cngError(fmt.Sprintf("delete key %q", ref.Label), status)
	}
	// NCryptDeleteKey frees the key handle on success.
	return nil
}

func (s cngSigner) loadKey(ctx context.Context, ref KeyRef) (windows.Handle, func(), error) {
	if err := validateRefContext(ctx, ref); err != nil {
		return 0, func() {}, err
	}
	provider, err := s.openProvider()
	if err != nil {
		return 0, func() {}, err
	}
	keyName, err := windows.UTF16PtrFromString(s.containerName(ref.Label))
	if err != nil {
		freeCNGObject(provider)
		return 0, func() {}, err
	}
	key, err := openCNGKey(provider, keyName)
	if err != nil {
		freeCNGObject(provider)
		return 0, func() {}, err
	}
	return key, func() {
		freeCNGObject(key)
		freeCNGObject(provider)
	}, nil
}

func (s cngSigner) openProvider() (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(s.providerName())
	if err != nil {
		return 0, err
	}
	var provider windows.Handle
	status, _, _ := ncryptOpenStorageProvider.Call(
		uintptr(unsafe.Pointer(&provider)),
		uintptr(unsafe.Pointer(name)),
		0,
	)
	if status != 0 {
		return 0, fmt.Errorf("%w: %w", ErrUnavailable, cngError("open key storage provider", status))
	}
	return provider, nil
}

func openCNGKey(provider windows.Handle, name *uint16) (windows.Handle, error) {
	var key windows.Handle
	status, _, _ := ncryptOpenKey.Call(
		uintptr(provider),
		uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(name)),
		0,
		cngSilentFlag,
	)
	if status != 0 {
		err := cngError("open key", status)
		if isCNGNotFound(err) {
			return 0, fmt.Errorf("%w: %w", ErrKeyNotFound, err)
		}
		return 0, err
	}
	return key, nil
}

func (s cngSigner) containerName(label string) string {
	if !s.software {
		return label
	}
	digest := sha256.Sum256([]byte(label))
	return "lark-cli-dpop-" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func isCNGNotFound(err error) bool {
	if err == nil {
		return false
	}
	var status cngStatusError
	if !errors.As(err, &status) {
		return false
	}
	return status.code == nteNotFound || status.code == nteBadKeyset
}

type cngStatusError struct {
	operation string
	code      uint32
}

func (e cngStatusError) Error() string {
	return fmt.Sprintf("keysigner: CNG %s failed with 0x%08X: %v", e.operation, e.code, syscall.Errno(e.code))
}

func (e cngStatusError) Unwrap() error { return syscall.Errno(e.code) }

// NCrypt returns SECURITY_STATUS directly; GetLastError may contain a stale success.
func cngError(operation string, status uintptr) error {
	err := cngStatusError{operation: operation, code: uint32(status)}
	switch uint32(status) {
	case 0x80090029, 0x80090030, 0x8028400F, 0x80290401:
		// NTE_NOT_SUPPORTED, NTE_DEVICE_NOT_READY, TBS_E_TPM_NOT_FOUND,
		// TPM_E_PCP_DEVICE_NOT_READY: preserve the previous L1 fallback contract.
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return err
}

func freeCNGObject(handle windows.Handle) {
	if handle != 0 {
		_, _, _ = ncryptFreeObject.Call(uintptr(handle))
	}
}

func (s cngSigner) providerName() string {
	if s.software {
		return cngSoftwareProvider
	}
	return cngPlatformProvider
}

func setCNGUint32(key windows.Handle, name string, value uint32) error {
	property, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	status, _, _ := ncryptSetProperty.Call(uintptr(key), uintptr(unsafe.Pointer(property)),
		uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value), cngPersistFlag)
	if status != 0 {
		return cngError("set "+name, status)
	}
	return nil
}

func exportCNGPublicKey(key windows.Handle, format string) ([]byte, error) {
	blobType, err := windows.UTF16PtrFromString(format)
	if err != nil {
		return nil, err
	}
	var size uint32
	status, _, _ := ncryptExportKey.Call(uintptr(key), 0, uintptr(unsafe.Pointer(blobType)), 0,
		0, 0, uintptr(unsafe.Pointer(&size)), 0)
	if status != 0 {
		return nil, cngError("measure public key", status)
	}
	if size == 0 || size > 16*1024 {
		return nil, ErrCorrupt
	}
	blob := make([]byte, size)
	status, _, _ = ncryptExportKey.Call(uintptr(key), 0, uintptr(unsafe.Pointer(blobType)), 0,
		uintptr(unsafe.Pointer(&blob[0])), uintptr(len(blob)), uintptr(unsafe.Pointer(&size)), 0)
	if status != 0 {
		return nil, cngError("export public key", status)
	}
	if size == 0 || size > uint32(len(blob)) {
		return nil, ErrCorrupt
	}
	return blob[:size], nil
}
