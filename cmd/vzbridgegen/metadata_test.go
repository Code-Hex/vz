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
	input, err := filepath.Abs("metadata/sdk.json")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := filepath.Abs("../../internal/vzbridge")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())
	output := filepath.Join(t.TempDir(), "generated")
	if err := run(input, output, "", false); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"arm64", "amd64"} {
		for _, name := range []string{"sdk_generated_" + arch + ".go", "sdk_contracts_generated_" + arch + "_test.go", "private_generated_" + arch + ".go"} {
			got, err := os.ReadFile(filepath.Join(output, name))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(reference, name))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from saved metadata; regenerate bindings", name)
			}
		}
	}
}

func TestMetadataRejectsStaleInputs(t *testing.T) {
	data, err := os.ReadFile("metadata/sdk.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, want string
		change     func(*generationMetadata)
	}{
		{"schema", "unsupported metadata schema", func(m *generationMetadata) { m.SDK.Schema = 2 }},
		{"extractor", "extractor changed", func(m *generationMetadata) { m.ExtractorSHA256 = "outdated" }},
		{"missing target", "arm64 and amd64", func(m *generationMetadata) { m.SDK.Targets = m.SDK.Targets[:1] }},
		{"duplicate target", "invalid metadata target", func(m *generationMetadata) { m.SDK.Targets[1].Architecture = m.SDK.Targets[0].Architecture }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var metadata generationMetadata
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			test.change(&metadata)
			dir := t.TempDir()
			input := filepath.Join(dir, "metadata.json")
			if err := writeMetadata(input, metadata); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "generated")
			err := run(input, output, "", false)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("invalid input created output: %v", err)
			}
		})
	}
}
