package vz_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Code-Hex/vz/v4"
)

func TestEFISecureBoot(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := vz.NewEFISignatureX509Certificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(certificate.Certificate(), der) {
		t.Fatal("certificate round trip differs")
	}
	path := filepath.Join(t.TempDir(), "certificate.der")
	if err := os.WriteFile(path, der, 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := vz.NewEFISignatureListFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if signatures := parsed.Signatures(); len(signatures) != 1 || !bytes.Equal(signatures[0].(*vz.EFISignatureX509Certificate).Certificate(), der) {
		t.Fatal("certificate file round trip differs")
	}
	hash := [32]byte{1, 2, 3}
	signature, err := vz.NewEFISignatureSHA256Hash(hash)
	if err != nil {
		t.Fatal(err)
	}
	if signature.Data() != hash {
		t.Fatal("hash round trip differs")
	}
	certs, err := vz.NewEFISignatureList([]vz.EFISignature{certificate})
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := vz.NewEFISignatureList([]vz.EFISignature{signature})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vz.NewEFISignatureDatabaseConfiguration([]*vz.EFISignatureList{hashes}, nil, nil); err == nil {
		t.Fatal("accepted SHA-256 key exchange key")
	}
	configuration, err := vz.NewEFISignatureDatabaseConfiguration([]*vz.EFISignatureList{certs}, []*vz.EFISignatureList{certs}, []*vz.EFISignatureList{hashes})
	if err != nil {
		t.Fatal(err)
	}
	store, err := vz.NewEFIVariableStore(filepath.Join(t.TempDir(), "efi"), vz.WithCreatingEFIVariableStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnrollSecureBootSignatures(configuration); err != nil {
		t.Fatal(err)
	}
	enrolled, err := store.EnrolledSecureBootSignatures()
	if err != nil {
		t.Fatal(err)
	}
	if len(enrolled.KeyExchangeKeys()) != 1 || len(enrolled.DbSignatures()) != 1 || len(enrolled.DbxSignatures()) != 1 {
		t.Fatal("signature databases did not round trip")
	}
	if got := enrolled.DbxSignatures()[0].Signatures()[0].(*vz.EFISignatureSHA256Hash).Data(); got != hash {
		t.Fatal("enrolled hash differs")
	}
	if got := enrolled.KeyExchangeKeys()[0].Signatures()[0].(*vz.EFISignatureX509Certificate).Certificate(); !bytes.Equal(got, der) {
		t.Fatal("enrolled certificate differs")
	}
	if err := store.EnableSecureBoot(certificate); err != nil {
		t.Fatal(err)
	}
	if enabled, err := store.SecureBootEnabled(); err != nil || !enabled {
		t.Fatalf("enabled = %v, error = %v", enabled, err)
	}
	if err := store.DisableSecureBoot(); err != nil {
		t.Fatal(err)
	}
	if enabled, err := store.SecureBootEnabled(); err != nil || enabled {
		t.Fatalf("enabled = %v, error = %v", enabled, err)
	}
	if err := store.EnrollDefaultSecureBootSignatures(); err != nil {
		t.Fatal(err)
	}
	if err := store.EnableSecureBootUsingDefaultPlatformKey(); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetSecureBoot(); err != nil {
		t.Fatal(err)
	}
	if enabled, err := store.SecureBootEnabled(); err != nil || enabled {
		t.Fatalf("enabled = %v, error = %v", enabled, err)
	}
}

func TestEFISignatureInvalidData(t *testing.T) {
	if vz.Available(27) {
		t.Skip("requires macOS 27")
	}
	if _, err := vz.NewEFISignatureX509Certificate([]byte("invalid")); err == nil {
		t.Fatal("accepted invalid certificate")
	}
	if _, err := vz.NewEFISignatureListFromFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted missing signature file")
	}
	for _, path := range []string{"missing\x00suffix", "\xff"} {
		if _, err := vz.NewEFISignatureListFromFile(path); err == nil {
			t.Fatalf("accepted invalid path %q", path)
		}
	}
}
