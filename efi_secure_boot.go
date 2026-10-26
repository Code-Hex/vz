package vz

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/Code-Hex/vz/v4/internal/objc"
	"github.com/Code-Hex/vz/v4/internal/objcutil"
	"github.com/Code-Hex/vz/v4/internal/vzbridge"
)

// EFISignature is a certificate or SHA-256 hash in a UEFI signature database.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignature-c.class?language=objc
type EFISignature interface {
	objc.NSObject
	efiSignature()
}

// EFISignatureX509Certificate is a DER-encoded X.509 Secure Boot signature.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturex509certificate?language=objc
type EFISignatureX509Certificate struct {
	*pointer
}

func (*EFISignatureX509Certificate) efiSignature() {}

// NewEFISignatureX509Certificate parses a DER-encoded certificate on macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturex509certificate/initwithcertificate:?language=objc
func NewEFISignatureX509Certificate(der []byte) (*EFISignatureX509Certificate, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	data := vzbridge.NSData_DataWithBytes_Length(unsafe.Pointer(unsafe.SliceData(der)), uint64(len(der)))
	runtime.KeepAlive(der)
	defer objc.Release(data)
	ptr := vzbridge.NewEFICertificate(data)
	if ptr == nil {
		return nil, fmt.Errorf("invalid DER-encoded X.509 certificate")
	}
	return &EFISignatureX509Certificate{
		pointer: ptr,
	}, nil
}

// Certificate returns a copy of the DER-encoded certificate.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturex509certificate/certificate?language=objc
func (s *EFISignatureX509Certificate) Certificate() []byte {
	return objcutil.Bytes(vzbridge.EfiCertificateData(s))
}

// EFISignatureSHA256Hash is a SHA-256 Secure Boot signature.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturesha256hash?language=objc
type EFISignatureSHA256Hash struct {
	*pointer
}

func (*EFISignatureSHA256Hash) efiSignature() {}

// NewEFISignatureSHA256Hash creates a signature on macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturesha256hash/initwithdata:?language=objc
func NewEFISignatureSHA256Hash(hash [32]byte) (*EFISignatureSHA256Hash, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	data := vzbridge.NSData_DataWithBytes_Length(unsafe.Pointer(&hash[0]), uint64(len(hash)))
	runtime.KeepAlive(hash)
	defer objc.Release(data)
	return &EFISignatureSHA256Hash{
		pointer: vzbridge.VZEFISignatureSHA256Hash_InitWithData(data),
	}, nil
}

// Data returns the signature's SHA-256 hash.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturesha256hash/data?language=objc
func (s *EFISignatureSHA256Hash) Data() [32]byte {
	return [32]byte(objcutil.Bytes(vzbridge.VZEFISignatureSHA256Hash_Data(s)))
}

// EFISignatureList contains X.509 certificates, SHA-256 hashes, or both.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturelist?language=objc
type EFISignatureList struct {
	*pointer
}

// NewEFISignatureList creates a signature list on macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturelist/initwithsignatures:?language=objc
func NewEFISignatureList(signatures []EFISignature) (*EFISignatureList, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	for _, signature := range signatures {
		switch s := signature.(type) {
		case *EFISignatureX509Certificate:
			if s == nil {
				return nil, fmt.Errorf("EFI signature must not be nil")
			}
		case *EFISignatureSHA256Hash:
			if s == nil {
				return nil, fmt.Errorf("EFI signature must not be nil")
			}
		}
		if objc.Ptr(signature) == nil {
			return nil, fmt.Errorf("EFI signature must not be nil")
		}
	}
	array := nativeObjectArray(signatures)
	defer objc.Release(array)
	return &EFISignatureList{
		pointer: vzbridge.VZEFISignatureList_InitWithSignatures(array),
	}, nil
}

// NewEFISignatureListFromFile reads a DER certificate, a binary UEFI signature list,
// or a SHA-256 hash from path. It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturelist/init(contentsofurl:)?language=objc
func NewEFISignatureListFromFile(path string) (*EFISignatureList, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	if !validUTF8CString(path) {
		return nil, fmt.Errorf("signature path must be valid UTF-8 without NUL bytes")
	}
	name := vzbridge.NSString_StringWithUTF8String(path)
	defer objc.Release(name)
	url := vzbridge.NSURL_FileURLWithPath(name)
	defer objc.Release(url)
	var nativeError unsafe.Pointer
	ptr := vzbridge.VZEFISignatureList_InitWithContentsOfURL_Error(url, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return nil, err
	}
	return &EFISignatureList{
		pointer: ptr,
	}, nil
}

// Signatures returns the entries in the list.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturelist/signatures-78pcm?language=objc
func (s *EFISignatureList) Signatures() []EFISignature {
	pointers := nativeArray(vzbridge.VZEFISignatureList_Signatures(s))
	signatures := make([]EFISignature, len(pointers))
	for i, ptr := range pointers {
		if vzbridge.IsEFICertificate(ptr) {
			signatures[i] = &EFISignatureX509Certificate{
				pointer: ptr,
			}
		} else {
			signatures[i] = &EFISignatureSHA256Hash{
				pointer: ptr,
			}
		}
	}
	return signatures
}

// EFISignatureDatabaseConfiguration holds the KEK, db, and dbx signature lists.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturedatabaseconfiguration?language=objc
type EFISignatureDatabaseConfiguration struct {
	*pointer
}

// NewEFISignatureDatabaseConfiguration creates Secure Boot databases on macOS 27 or later.
// Key exchange keys must contain only X.509 certificates. Empty databases are allowed.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturedatabaseconfiguration/init(keyexchangekeys:dbsignatures:dbxsignatures:)?language=objc
func NewEFISignatureDatabaseConfiguration(keyExchangeKeys, dbSignatures, dbxSignatures []*EFISignatureList) (*EFISignatureDatabaseConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	for _, lists := range [][]*EFISignatureList{keyExchangeKeys, dbSignatures, dbxSignatures} {
		for _, list := range lists {
			if list == nil || objc.Ptr(list) == nil {
				return nil, fmt.Errorf("EFI signature list must not be nil")
			}
		}
	}
	for _, list := range keyExchangeKeys {
		for _, signature := range list.Signatures() {
			if _, ok := signature.(*EFISignatureX509Certificate); !ok {
				return nil, fmt.Errorf("key exchange keys must contain only X.509 certificates")
			}
		}
	}
	kek := nativeObjectArray(keyExchangeKeys)
	defer objc.Release(kek)
	db := nativeObjectArray(dbSignatures)
	defer objc.Release(db)
	dbx := nativeObjectArray(dbxSignatures)
	defer objc.Release(dbx)
	return &EFISignatureDatabaseConfiguration{
		pointer: vzbridge.VZEFISignatureDatabaseConfiguration_InitWithKeyExchangeKeys_DbSignatures_DbxSignatures(kek, db, dbx),
	}, nil
}

func efiSignatureLists(array *pointer) []*EFISignatureList {
	pointers := nativeArray(array)
	lists := make([]*EFISignatureList, len(pointers))
	for i, ptr := range pointers {
		lists[i] = &EFISignatureList{
			pointer: ptr,
		}
	}
	return lists
}

// KeyExchangeKeys returns the Key Exchange Key (KEK) database entries.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturedatabaseconfiguration/keyexchangekeys?language=objc
func (c *EFISignatureDatabaseConfiguration) KeyExchangeKeys() []*EFISignatureList {
	return efiSignatureLists(vzbridge.VZEFISignatureDatabaseConfiguration_KeyExchangeKeys(c))
}

// DbSignatures returns the allowed signature (db) database entries.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturedatabaseconfiguration/dbsignatures?language=objc
func (c *EFISignatureDatabaseConfiguration) DbSignatures() []*EFISignatureList {
	return efiSignatureLists(vzbridge.VZEFISignatureDatabaseConfiguration_DbSignatures(c))
}

// DbxSignatures returns the forbidden signature (dbx) database entries.
//
// see: https://developer.apple.com/documentation/virtualization/vzefisignaturedatabaseconfiguration/dbxsignatures?language=objc
func (c *EFISignatureDatabaseConfiguration) DbxSignatures() []*EFISignatureList {
	return efiSignatureLists(vzbridge.VZEFISignatureDatabaseConfiguration_DbxSignatures(c))
}

// EnableSecureBootUsingDefaultPlatformKey enables Secure Boot with an Apple-managed Platform Key.
// It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/enablesecurebootusingdefaultplatformkey()?language=objc
func (e *EFIVariableStore) EnableSecureBootUsingDefaultPlatformKey() error {
	if err := macOSAvailable(27); err != nil {
		return err
	}
	var nativeError unsafe.Pointer
	vzbridge.VZEFIVariableStore_EnableSecureBootUsingDefaultPlatformKeyWithError(e, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return err
	}
	return nil
}

// DisableSecureBoot disables Secure Boot while preserving its configuration.
// It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/disablesecureboot()?language=objc
func (e *EFIVariableStore) DisableSecureBoot() error {
	if err := macOSAvailable(27); err != nil {
		return err
	}
	var nativeError unsafe.Pointer
	vzbridge.VZEFIVariableStore_DisableSecureBootWithError(e, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return err
	}
	return nil
}

// ResetSecureBoot clears the Secure Boot configuration and disables Secure Boot.
// It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/resetsecureboot()?language=objc
func (e *EFIVariableStore) ResetSecureBoot() error {
	if err := macOSAvailable(27); err != nil {
		return err
	}
	var nativeError unsafe.Pointer
	vzbridge.VZEFIVariableStore_ResetSecureBootWithError(e, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return err
	}
	return nil
}

// EnrollDefaultSecureBootSignatures adds Microsoft KEK, UEFI CA, and revocation signatures.
// It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/enrolldefaultsecurebootsignatures()?language=objc
func (e *EFIVariableStore) EnrollDefaultSecureBootSignatures() error {
	if err := macOSAvailable(27); err != nil {
		return err
	}
	var nativeError unsafe.Pointer
	vzbridge.VZEFIVariableStore_EnrollDefaultSecureBootSignaturesWithError(e, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return err
	}
	return nil
}

// EnableSecureBoot enables Secure Boot with a custom Platform Key on macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/enablesecureboot(platformkey:)?language=objc
func (e *EFIVariableStore) EnableSecureBoot(platformKey *EFISignatureX509Certificate) error {
	if err := macOSAvailable(27); err != nil {
		return err
	}
	if platformKey == nil || objc.Ptr(platformKey) == nil {
		return fmt.Errorf("platform key must not be nil")
	}
	var nativeError unsafe.Pointer
	vzbridge.EnableEFISecureBoot(e, platformKey, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return err
	}
	return nil
}

// SecureBootEnabled reports whether Secure Boot is enabled on macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/getsecurebootenabled:error:?language=objc
func (e *EFIVariableStore) SecureBootEnabled() (bool, error) {
	if err := macOSAvailable(27); err != nil {
		return false, err
	}
	var nativeError unsafe.Pointer
	enabled := vzbridge.EfiSecureBootEnabled(e, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return false, err
	}
	return enabled, nil
}

// EnrollSecureBootSignatures appends signatures to the KEK, db, and dbx databases.
// Existing signatures and the Platform Key are preserved. It requires macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/enrollsecurebootsignatures(_:)?language=objc
func (e *EFIVariableStore) EnrollSecureBootSignatures(signatures *EFISignatureDatabaseConfiguration) error {
	if err := macOSAvailable(27); err != nil {
		return err
	}
	if signatures == nil || objc.Ptr(signatures) == nil {
		return fmt.Errorf("signature databases must not be nil")
	}
	var nativeError unsafe.Pointer
	vzbridge.VZEFIVariableStore_EnrollSecureBootSignatures_Error(e, signatures, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return err
	}
	return nil
}

// EnrolledSecureBootSignatures returns the current databases on macOS 27 or later.
//
// see: https://developer.apple.com/documentation/virtualization/vzefivariablestore/getenrolledsecurebootsignatureswitherror:?language=objc
func (e *EFIVariableStore) EnrolledSecureBootSignatures() (*EFISignatureDatabaseConfiguration, error) {
	if err := macOSAvailable(27); err != nil {
		return nil, err
	}
	var nativeError unsafe.Pointer
	ptr := vzbridge.VZEFIVariableStore_GetEnrolledSecureBootSignaturesWithError(e, &nativeError)
	if err := objcutil.NewNSError(nativeError); err != nil {
		return nil, err
	}
	return &EFISignatureDatabaseConfiguration{
		pointer: ptr,
	}, nil
}
