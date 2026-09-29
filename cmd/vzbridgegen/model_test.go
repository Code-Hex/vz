package main

import (
	"strings"
	"testing"
)

func TestCompilerContract(t *testing.T) {
	ast := `{"kind":"FunctionDecl","name":"vz_create","type":{"qualType":"void *(void *, int32_t)"},"inner":[{"kind":"ParmVarDecl","name":"parent","type":{"qualType":"void *"}},{"kind":"ParmVarDecl","name":"count","type":{"qualType":"int32_t"}}]}`
	operations := []operation{{Symbol: "vz_create", Name: "Create", Parameters: map[string]string{"parent": "object"}, Result: "owned"}}
	got, err := checkHeader(strings.NewReader(ast), operations, "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Parameters[0].Go != "objc.NSObject" || got[0].Parameters[1].Go != "int32" || got[0].Result.Go != "*objc.Pointer" || got[0].Result.C != "void *" || got[0].Parameters[1].C != "int32_t" {
		t.Fatalf("unexpected signature: %+v", got)
	}
	operations[0].Parameters = nil
	if _, err := checkHeader(strings.NewReader(ast), operations, "arm64"); err == nil {
		t.Fatal("unclassified pointer accepted")
	}
}

func TestRejectInvalidContract(t *testing.T) {
	ast := `{"kind":"FunctionDecl","name":"vz_value","type":{"qualType":"int32_t (int32_t)"},"inner":[{"kind":"ParmVarDecl","name":"value","type":{"qualType":"int32_t"}}]}`
	for _, contract := range []operation{
		{Symbol: "vz_missing", Name: "Missing"},
		{Symbol: "vz_value", Name: "Value", Parameters: map[string]string{"missing": "raw"}},
		{Symbol: "vz_value", Name: "Value", Parameters: map[string]string{"value": "object"}},
		{Symbol: "vz_value", Name: "Value", Result: "owned"},
	} {
		if _, err := checkHeader(strings.NewReader(ast), []operation{contract}, "arm64"); err == nil {
			t.Fatalf("invalid contract accepted: %+v", contract)
		}
	}
}

func TestSDKVersionConstant(t *testing.T) {
	for input, want := range map[string]int{"27.0": 270000, "15.4.1": 150401, "11": 110000} {
		got, err := sdkVersionConstant(input)
		if err != nil || got != want {
			t.Fatalf("%s: got %d, %v", input, got, err)
		}
	}
	for _, input := range []string{"", "27.a", "-1.0", "27.100", "27.0.0.1"} {
		if _, err := sdkVersionConstant(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestRejectBorrowedCStringResult(t *testing.T) {
	ast := `{"kind":"FunctionDecl","name":"vz_text","type":{"qualType":"const char *(void)"}}`
	if _, err := checkHeader(strings.NewReader(ast), []operation{{Symbol: "vz_text", Name: "Text", Result: "cstring"}}, "arm64"); err == nil {
		t.Fatal("borrowed CString result accepted without an ownership lifetime")
	}
}

func TestCompilerCType(t *testing.T) {
	got, err := translate("const char * _Nullable", "cstring", false)
	if err != nil || got.C != "const char *" || got.Go != "string" {
		t.Fatalf("qualified pointer: got %+v, %v", got, err)
	}
	for _, typ := range []string{"long", "int64_t", "intptr_t"} {
		got, err := translate(typ, "", false)
		if err != nil || got.C != typ || got.Go != "int64" {
			t.Fatalf("%s: got %+v, %v", typ, got, err)
		}
	}
}
