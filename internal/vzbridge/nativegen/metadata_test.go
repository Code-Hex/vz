package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataReplay(t *testing.T) {
	input, err := filepath.Abs("../../../cmd/vzbridgegen/metadata/native.json")
	if err != nil {
		t.Fatal(err)
	}
	header, err := filepath.Abs("../native.h")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../native_bindings.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())
	output := filepath.Join(t.TempDir(), "native_bindings.go")
	if err := run(input, header, output, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("native_bindings.go differs from saved metadata; regenerate bindings")
	}
}

func TestMetadataRejectsStaleInputs(t *testing.T) {
	data, err := os.ReadFile("../../../cmd/vzbridgegen/metadata/native.json")
	if err != nil {
		t.Fatal(err)
	}
	header, err := os.ReadFile("../native.h")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, want string
		change     func(*generationMetadata)
	}{
		{"schema", "unsupported metadata schema", func(m *generationMetadata) { m.Schema = 2 }},
		{"extractor", "extractor changed", func(m *generationMetadata) { m.ExtractorSHA256 = "outdated" }},
		{"header", "native header changed", func(m *generationMetadata) { m.HeaderSHA256 = "outdated" }},
		{"empty", "no native bindings", func(m *generationMetadata) { m.Bindings = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var metadata generationMetadata
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			test.change(&metadata)
			changed, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			input, source, output := filepath.Join(dir, "metadata.json"), filepath.Join(dir, "native.h"), filepath.Join(dir, "native_bindings.go")
			if err := os.WriteFile(input, changed, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, header, 0600); err != nil {
				t.Fatal(err)
			}
			err = run(input, source, output, false)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid input created output: %v", err)
			}
		})
	}
}
