package main

import (
	"bytes"
	"testing"
)

func TestSDKContractsTrackCompilerABI(t *testing.T) {
	target := compileFixture(t, fixture)
	code, err := generateContracts(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"VZFixture_Score"`, `ResultABI: "int64"`, `"VZFixture_ValidateWithError"`, `"*unsafe.Pointer"`} {
		if !bytes.Contains(code, []byte(want)) {
			t.Errorf("contract missing %s", want)
		}
	}
	if bytes.Contains(code, []byte(`"VZFixture_Consume"`)) {
		t.Fatal("unsupported method became a runtime contract")
	}
}
