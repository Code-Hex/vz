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
	Schema          int
	ExtractorSHA256 string
	HeaderSHA256    string
	Bindings        []binding
}

//go:embed header.go
var extractorSource string

func extractorSHA256() string { return fmt.Sprintf("%x", sha256.Sum256([]byte(extractorSource))) }

func extractMetadata(path, header string) error {
	bindings, err := discoverBindings(header)
	if err != nil {
		return err
	}
	source, err := os.ReadFile(header)
	if err != nil {
		return err
	}
	metadata := generationMetadata{Schema: 1, ExtractorSHA256: extractorSHA256(), HeaderSHA256: fmt.Sprintf("%x", sha256.Sum256(source)), Bindings: bindings}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func readMetadata(path, header string) (generationMetadata, error) {
	var metadata generationMetadata
	data, err := os.ReadFile(path)
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, err
	}
	if metadata.Schema != 1 {
		return metadata, fmt.Errorf("unsupported metadata schema %d", metadata.Schema)
	}
	if metadata.ExtractorSHA256 != extractorSHA256() {
		return metadata, fmt.Errorf("extractor changed; refresh metadata with -extract")
	}
	source, err := os.ReadFile(header)
	if err != nil {
		return metadata, err
	}
	if metadata.HeaderSHA256 != fmt.Sprintf("%x", sha256.Sum256(source)) {
		return metadata, fmt.Errorf("native header changed; refresh metadata with -extract")
	}
	if len(metadata.Bindings) == 0 {
		return metadata, fmt.Errorf("metadata contains no native bindings")
	}
	return metadata, nil
}
