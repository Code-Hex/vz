package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureTypes = `
typedef void *Object __attribute__((annotate("vzbridge:object")));
typedef void *Owned __attribute__((annotate("vzbridge:owned")));
typedef void **ErrorOut __attribute__((annotate("vzbridge:errorout")));
`

func fixtureBindings(t *testing.T, source string) ([]binding, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "native.h")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return discoverBindings(path)
}

func TestHeaderChangesBindingTypes(t *testing.T) {
	for _, test := range []struct{ ctype, gotype string }{
		{"unsigned int", "uint32"}, {"unsigned long long", "uint64"}, {"float", "float32"}, {"double", "float64"},
	} {
		t.Run(test.gotype, func(t *testing.T) {
			bindings, err := fixtureBindings(t, "typedef "+test.ctype+" Sample; Sample vz_sample(Sample value);")
			if err != nil {
				t.Fatal(err)
			}
			if len(bindings) != 1 || bindings[0].Result != test.gotype || bindings[0].Parameters[0].Type != test.gotype {
				t.Fatalf("header type was not reflected in bindings: %#v", bindings)
			}
			source, err := generate(bindings)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), "func Sample(value "+test.gotype+") "+test.gotype) {
				t.Fatalf("generated signature does not follow header:\n%s", source)
			}
		})
	}
}

func TestHeaderOwnsObjectAndQueueMetadata(t *testing.T) {
	bindings, err := fixtureBindings(t, fixtureTypes+`
Owned vz_create(Object input, const char *name, ErrorOut error);
void vz_release(void *object) __attribute__((annotate("vzbridge:noqueue")));
void vz_dispatch(unsigned long callback) __attribute__((annotate("vzbridge:manual")));
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 {
		t.Fatalf("got %d bindings, want 2", len(bindings))
	}
	create, release := bindings[0], bindings[1]
	if create.Name != "Create" || create.Result != "*objc.Pointer" || !create.Queue || release.Queue {
		t.Fatalf("ownership or queue metadata lost: %#v", bindings)
	}
	for i, want := range []string{"objc.NSObject", "string", "*unsafe.Pointer"} {
		if create.Parameters[i].Type != want {
			t.Fatalf("parameter %d = %s, want %s", i, create.Parameters[i].Type, want)
		}
	}
}

func TestHeaderRejectsInvalidContracts(t *testing.T) {
	for _, test := range []struct{ name, source, message string }{
		{"metadata typo", `void vz_call(void) __attribute__((annotate("vzbridge:noqeue")));`, "unknown annotation"},
		{"owned scalar", `typedef int Owned __attribute__((annotate("vzbridge:owned"))); Owned vz_call(void);`, "requires void *"},
		{"errorout scalar", `typedef int ErrorOut __attribute__((annotate("vzbridge:errorout"))); void vz_call(ErrorOut error);`, "requires void * *"},
		{"errorout result", fixtureTypes + `ErrorOut vz_call(void);`, "errorout type is only valid as a parameter"},
		{"owned input", fixtureTypes + `void vz_call(Owned value);`, "owned type is only valid as a result"},
		{"borrowed output", fixtureTypes + `Object vz_call(void);`, "object type is only valid as a parameter"},
		{"unknown pointer", `struct Unknown; void vz_call(struct Unknown *value);`, "unsupported native ABI type"},
		{"calling convention", `void vz_call(void) __attribute__((ms_abi));`, "unsupported function type"},
		{"function pointer", `void vz_call(void (*callback)(void));`, "unsupported native ABI type"},
		{"variadic", `void vz_call(int value, ...);`, "variadic"},
		{"duplicate symbol", `void vz_call(void); void vz_call(void);`, "duplicate native symbol"},
		{"duplicate name", `void vz_call(void); void vz_Call(void);`, "duplicate Go name"},
		{"parameter annotation", `void vz_call(void *value __attribute__((annotate("vzbridge:noqueue"))));`, "parameter annotation"},
		{"conflicting queue", `void vz_call(void) __attribute__((annotate("vzbridge:noqueue"), annotate("vzbridge:manual")));`, "multiple annotations"},
		{"missing argument name", `void vz_call(int);`, "invalid parameter"},
		{"syntax error", `void vz_call( ;`, "clang"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fixtureBindings(t, test.source)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestHeaderRejectsIncludedAnnotations(t *testing.T) {
	dir := t.TempDir()
	foreign := `typedef void *Foreign __attribute__((annotate("vzbridge:xxxxx")));`
	if err := os.WriteFile(filepath.Join(dir, "foreign.h"), []byte(foreign), 0600); err != nil {
		t.Fatal(err)
	}
	offset := strings.Index(foreign, `annotate("vzbridge:xxxxx")`)
	source := "#include \"foreign.h\"\n/*"
	source += strings.Repeat(" ", offset-len(source)) + `annotate("vzbridge:owned")` + "*/\nForeign vz_bad(void);\n"
	path := filepath.Join(dir, "native.h")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverBindings(path); err == nil || !strings.Contains(err.Error(), "annotations must be written directly in the header") {
		t.Fatalf("included annotation was not rejected: %v", err)
	}
}

func TestHeaderRejectsEmptyInventory(t *testing.T) {
	for _, source := range []string{"typedef int Sample;", `void vz_dispatch(void) __attribute__((annotate("vzbridge:manual")));`} {
		if _, err := fixtureBindings(t, source); err == nil || !strings.Contains(err.Error(), "no native bindings") {
			t.Fatalf("empty inventory was not rejected: %v", err)
		}
	}
}

func TestHeaderAnnotationOffsetsAreBytes(t *testing.T) {
	bindings, err := fixtureBindings(t, "/* \u65e5\u672c\u8a9e */\n"+fixtureTypes+"Owned vz_create(Object input);")
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Result != "*objc.Pointer" || bindings[0].Parameters[0].Type != "objc.NSObject" {
		t.Fatalf("annotation metadata lost after UTF-8 text: %#v", bindings)
	}
}

func TestGeneratorDoesNotOverwriteHeader(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "native.h"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile("native.h", source, 0600); err != nil {
		t.Fatal(err)
	}
	main()
	after, err := os.ReadFile("native.h")
	if err != nil {
		t.Fatal(err)
	}
	if string(source) != string(after) {
		t.Fatal("generation overwrote the source header")
	}
}
