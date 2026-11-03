package guiexample

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewAndBootOptions(t *testing.T) {
	preview, err := parseOptions(nil)
	if err != nil || preview.iso != "" || preview.Autostart {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	iso := filepath.Join(t.TempDir(), "linux.iso")
	if err := os.WriteFile(iso, nil, 0600); err != nil {
		t.Fatal(err)
	}
	boot, err := parseOptions([]string{"-iso", iso, "-autostart", "-capture-keys", "-auto-resize"})
	if err != nil || boot.iso != iso || !boot.Autostart || !boot.CaptureKeys || !boot.AutoResize {
		t.Fatalf("boot: %+v, %v", boot, err)
	}
}

func TestInvalidOptions(t *testing.T) {
	for _, args := range [][]string{{"-autostart"}, {"-iso", "/no/such/linux.iso"}, {"-iso", t.TempDir()}, {"-disk", "/no/such/disk"}, {"unexpected"}, {"-unknown"}} {
		t.Run(args[0], func(t *testing.T) {
			if _, err := parseOptions(args); err == nil {
				t.Fatalf("accepted %v", args)
			}
		})
	}
}

func TestSerialLogCannotOverwriteBootFiles(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "guest.img")
	if err := os.WriteFile(source, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	symlink, hardlink := filepath.Join(dir, "symbolic"), filepath.Join(dir, "hard")
	if err := os.Symlink(source, symlink); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, hardlink); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"-iso", "-disk"} {
		for _, output := range []string{source, symlink, hardlink} {
			if _, err := parseOptions([]string{input, source, "-serial-log", output}); err == nil {
				t.Errorf("accepted %s with aliased serial output %s", input, output)
			}
		}
	}
}

func TestWritableDiskCannotAliasISO(t *testing.T) {
	dir := t.TempDir()
	iso := filepath.Join(dir, "linux.iso")
	if err := os.WriteFile(iso, []byte("installation media"), 0600); err != nil {
		t.Fatal(err)
	}
	for name, link := range map[string]func(string, string) error{"symlink": os.Symlink, "hardlink": os.Link} {
		t.Run(name, func(t *testing.T) {
			disk := filepath.Join(dir, name)
			if err := link(iso, disk); err != nil {
				t.Fatal(err)
			}
			if _, err := parseOptions([]string{"-iso", iso, "-disk", disk}); err == nil {
				t.Fatal("accepted ISO image as writable disk")
			}
		})
	}
	if _, err := parseOptions([]string{"-iso", iso, "-disk", iso}); err == nil {
		t.Fatal("accepted ISO image as writable disk")
	}
	disk := filepath.Join(dir, "disk.raw")
	if err := os.WriteFile(disk, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseOptions([]string{"-iso", iso, "-disk", disk}); err != nil {
		t.Fatalf("distinct ISO and disk: %v", err)
	}
}
