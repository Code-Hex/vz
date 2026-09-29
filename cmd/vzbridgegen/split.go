package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed split.swift
var swiftSplitter []byte

func splitSwiftSources(sources, symbols []string, directory string) ([]string, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, err
	}
	compiler, err := command("xcrun", "--find", "swiftc")
	if err != nil {
		return nil, err
	}
	host := filepath.Join(filepath.Dir(strings.TrimSpace(string(compiler))), "..", "lib", "swift", "host")
	source := filepath.Join(directory, "Split.swift")
	if err := os.WriteFile(source, swiftSplitter, 0600); err != nil {
		return nil, err
	}
	binary := filepath.Join(directory, "split")
	if _, err := command("xcrun", "swiftc", "-swift-version", "6", "-strict-concurrency=complete", "-warnings-as-errors", "-I", host, "-L", host, "-Xlinker", "-rpath", "-Xlinker", host, source, "-o", binary); err != nil {
		return nil, err
	}
	request, err := json.Marshal(struct {
		Sources []string `json:"sources"`
		Symbols []string `json:"symbols"`
		Output  string   `json:"output"`
	}{sources, symbols, directory})
	if err != nil {
		return nil, err
	}
	input := filepath.Join(directory, "input.json")
	if err := os.WriteFile(input, request, 0600); err != nil {
		return nil, err
	}
	data, err := command(binary, input)
	if err != nil {
		return nil, err
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("split Swift sources: %w", err)
	}
	return paths, nil
}
