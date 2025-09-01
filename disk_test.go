package vz_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Code-Hex/vz/v3"
)

func TestCreateSparseDiskImage_FileCreated(t *testing.T) {
	if vz.Available(26) {
		t.Skip("CreateSparseDiskImage is supported from macOS 26")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sparse_disk.img")

	ctx := context.Background()
	size := int64(1024 * 1024 * 1024) // 1 GiB

	err := vz.CreateSparseDiskImage(ctx, path, size)
	if err != nil {
		t.Fatalf("failed to create sparse disk image: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("disk image file was not created")
	}
}

func TestCreateSparseDiskImage_ASIFFormat(t *testing.T) {
	if vz.Available(26) {
		t.Skip("CreateSparseDiskImage is supported from macOS 26")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sparse_disk.img")

	ctx := context.Background()
	size := int64(1024 * 1024 * 1024) // 1 GiB

	err := vz.CreateSparseDiskImage(ctx, path, size)
	if err != nil {
		t.Fatalf("failed to create sparse disk image: %v", err)
	}

	// Check if the format is ASIF using diskutil
	cmd := exec.Command("diskutil", "image", "info", path)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get disk image info: %v", err)
	}

	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	foundASIF := false
	// Check if ASIF is mentioned in the first line
	if len(lines) != 0 && strings.Contains(lines[0], "ASIF") {
		foundASIF = true
	}

	if !foundASIF {
		t.Errorf("disk image is not in ASIF format. Output: %v", lines[:1])
	}
}

func TestCreateSparseDiskImage_CorrectSize(t *testing.T) {
	if vz.Available(26) {
		t.Skip("CreateSparseDiskImage is supported from macOS 26")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "sparse_disk.img")

	ctx := context.Background()
	desiredSize := int64(2 * 1024 * 1024 * 1024) // 2 GiB

	err := vz.CreateSparseDiskImage(ctx, path, desiredSize)
	if err != nil {
		t.Fatalf("failed to create sparse disk image: %v", err)
	}

	cmd := exec.Command("diskutil", "image", "info", path)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get disk image info: %v", err)
	}

	var sizeStr string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "Total Bytes") {
			components := strings.Split(strings.TrimSpace(line), ":")
			if len(components) > 1 {
				sizeStr = strings.TrimSpace(components[1])
				break
			}
		}
	}
	actualSize, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		t.Fatalf("failed to parse string to int: %v", err)
	}

	if desiredSize != actualSize {
		t.Fatalf("actual disk size (%d) doesn't equal to desired size (%d)", actualSize, desiredSize)
	}
}
