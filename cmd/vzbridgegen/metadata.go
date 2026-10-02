package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type generationMetadata struct {
	ExtractorSHA256 string
	SDK             sdkReport
	Private         privateMetadata
}

//go:embed extract.go
var sdkExtractorSource string

//go:embed model.go
var sdkModelSource string

//go:embed private_extract.go
var runtimeExtractorSource string

func extractorSHA256() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(sdkExtractorSource+"\x00"+sdkModelSource+"\x00"+runtimeExtractorSource)))
}

func readMetadata(path string) (generationMetadata, error) {
	var metadata generationMetadata
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, err
	}
	if metadata.SDK.Schema != 1 {
		return metadata, fmt.Errorf("unsupported metadata schema %d", metadata.SDK.Schema)
	}
	if metadata.ExtractorSHA256 != extractorSHA256() {
		return metadata, fmt.Errorf("extractor changed; refresh metadata with -extract")
	}
	seen := map[string]bool{}
	for _, target := range metadata.SDK.Targets {
		if target.Architecture != "arm64" && target.Architecture != "amd64" || seen[target.Architecture] {
			return metadata, fmt.Errorf("invalid metadata target %q", target.Architecture)
		}
		seen[target.Architecture] = true
	}
	if len(seen) != 2 {
		return metadata, fmt.Errorf("metadata must contain arm64 and amd64 targets")
	}
	return metadata, nil
}

func writeMetadata(path string, metadata generationMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
