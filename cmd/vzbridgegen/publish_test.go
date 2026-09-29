package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishPreservesUnexpectedFiles(t *testing.T) {
	destination := t.TempDir()
	bundle := filepath.Join(destination, "abi_arm64")
	if err := os.Mkdir(bundle, 0755); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(bundle, "local.swift")
	if err := os.WriteFile(private, []byte("local work"), 0644); err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	for _, name := range []string{"abi_arm64/libBridge.a", "abi_amd64/libBridge.a", "abi_arm64/Virtualization.tbd", "abi_amd64/Virtualization.tbd", "binding_arm64.go", "binding_amd64.go", "abi_arm64/Framework.swift", "abi_amd64/Framework.swift"} {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("generated"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := publishBuild(source, destination); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"binding_arm64.go", "binding_amd64.go"} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(data) != "generated" {
			t.Fatalf("missing generated binding: %s, %v", data, err)
		}
	}
	content, err := os.ReadFile(private)
	if err != nil || string(content) != "local work" {
		t.Fatalf("local work changed: %s, %v", content, err)
	}
}

func TestPublishRefusesHandwrittenBinding(t *testing.T) {
	destination := t.TempDir()
	loader := filepath.Join(destination, "binding_arm64.go")
	if err := os.WriteFile(loader, []byte("package local"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := publishBuild(t.TempDir(), destination); err == nil {
		t.Fatal("handwritten loader overwritten")
	}
	content, err := os.ReadFile(loader)
	if err != nil || string(content) != "package local" {
		t.Fatalf("local loader changed: %s, %v", content, err)
	}
}
