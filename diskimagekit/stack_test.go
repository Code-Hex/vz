package diskimagekit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheParentIdentity(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.raw")
	cache := filepath.Join(dir, "cache.asif")
	overlay := filepath.Join(dir, "overlay.asif")
	d, err := CreateRAW(base, 8192)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteAt([]byte("parent"), 0); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateLayer(cache, Cache); err != nil {
		t.Fatal(err)
	}
	cacheID := d.LayerUUID()
	if err := d.CreateLayer(overlay, Overlay); err != nil {
		t.Fatal(err)
	}
	if d.ParentUUID() != "" {
		t.Fatalf("overlay on RAW has parent UUID %q", d.ParentUUID())
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(base)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Append(cache, WithOpenMode(ReadWrite)); err != nil {
		t.Fatal(err)
	}
	if d.LayerUUID() != cacheID {
		t.Fatal("stack getter skipped the cache UUID")
	}
	if err := d.Append(overlay); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 6)
	if _, err := d.ReadAt(got, 0); err != nil || string(got) != "parent" {
		t.Fatalf("inherited read = %q, %v", got, err)
	}
}

func TestDetachedStackBase(t *testing.T) {
	for _, kind := range []LayerType{Overlay, Cache} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.asif")
			layerPath := filepath.Join(dir, "layer.asif")
			child := filepath.Join(dir, "child.asif")
			d, err := CreateASIF(base, 8192)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.CreateLayer(layerPath, kind); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(child, Overlay); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			detached, err := Open(layerPath)
			if err != nil {
				t.Fatal(err)
			}
			defer detached.Close()
			if err := detached.Append(child); !errors.Is(err, ErrIncompatibleStack) {
				t.Fatalf("append on detached base = %v", err)
			}
			destination := filepath.Join(dir, "new.asif")
			if err := detached.CreateLayer(destination, Overlay); !errors.Is(err, ErrIncompatibleStack) {
				t.Fatalf("create on detached base = %v", err)
			}
			if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected creation left a file: %v", err)
			}
			if len(detached.Layers()) != 1 {
				t.Fatal("rejected append changed the stack")
			}
		})
	}
}

func TestLayerUUIDValidation(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprint(empty), func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.asif")
			cache := filepath.Join(dir, "cache.asif")
			d, err := CreateASIF(base, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(cache, Cache); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			id := make([]byte, 16)
			if !empty {
				id[0] = 1
			}
			f, err := os.OpenFile(cache, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.WriteAt(id, 0x20)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			standalone, err := Open(cache)
			if err != nil {
				t.Fatal(err)
			}
			if (standalone.LayerUUID() == "") != empty {
				t.Fatal("standalone UUID differs")
			}
			standalone.Close()
			d, err = Open(base)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			err = d.Append(cache)
			if empty && !errors.Is(err, ErrIncompatibleStack) || !empty && err != nil {
				t.Fatalf("append = %v", err)
			}
		})
	}
}

func TestOverlayPreservesInheritedSectors(t *testing.T) {
	for _, block := range []BlockSize{BlockSize512, BlockSize4096} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.asif")
			cache := filepath.Join(dir, "cache.asif")
			overlay := filepath.Join(dir, "overlay.asif")
			d, err := CreateASIF(base, (4<<20)/int64(block), WithBlockSize(block))
			if err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(cache, Cache); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(overlay, Overlay); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			openStack := func() *DiskImage {
				t.Helper()
				d, err := Open(base, WithOpenMode(ReadOnly))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { d.Close() })
				if err := d.Append(cache, WithOpenMode(ReadOnly)); err != nil {
					t.Fatal(err)
				}
				if err := d.Append(overlay, WithOpenMode(ReadWrite)); err != nil {
					t.Fatal(err)
				}
				return d
			}
			d = openStack()
			want := bytes.Repeat([]byte{'W'}, int(block))
			for _, off := range []int64{0, 1 << 20} {
				if _, err := d.WriteAt(want, off); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.Truncate((1<<20)/int64(block) + 2); err != nil {
				t.Fatal(err)
			}
			if err := d.Truncate((4 << 20) / int64(block)); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d = openStack()
			got := make([]byte, block)
			for _, off := range []int64{0, 1 << 20} {
				if _, err := d.ReadAt(got, off); err != nil || !bytes.Equal(got, want) {
					t.Fatalf("written sector at %d: %v", off, err)
				}
				if _, err := d.ReadAt(got, off+int64(block)); !errors.Is(err, ErrReadOnly) {
					t.Fatalf("inherited sector at %d: %v", off, err)
				}
			}
			if _, err := d.ReadAt(got, (1<<20)+2*int64(block)); err != nil || !bytes.Equal(got, make([]byte, block)) {
				t.Fatalf("discarded sector: %v", err)
			}
		})
	}
}

func TestCreateLayerPreservesOwnedFiles(t *testing.T) {
	for _, target := range []string{"base", "top", "hardlink", "symlink", "other"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			base, top := filepath.Join(dir, "base.asif"), filepath.Join(dir, "top.asif")
			d, err := CreateASIF(base, 8)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if _, err := d.WriteAt([]byte("parent"), 0); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(top, Overlay); err != nil {
				t.Fatal(err)
			}
			before := make(map[string][]byte)
			for _, path := range []string{base, top} {
				before[path], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(dir, target)
			switch target {
			case "base":
				destination = base
			case "top":
				destination = top
			case "hardlink":
				err = os.Link(base, destination)
			case "symlink":
				err = os.Symlink(base, destination)
			case "other":
				err = os.WriteFile(destination, []byte("replace me"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = d.CreateLayer(destination, Overlay)
			wantLayers := 2
			if target == "other" {
				wantLayers = 3
				if err != nil {
					t.Fatalf("replacing another file: %v", err)
				}
			} else if !errors.Is(err, ErrIncompatibleStack) {
				t.Fatalf("replacing an owned file: %v", err)
			}
			if len(d.Layers()) != wantLayers {
				t.Fatalf("layer count = %d, want %d", len(d.Layers()), wantLayers)
			}
			got := make([]byte, 6)
			if _, err := d.ReadAt(got, 0); err != nil || string(got) != "parent" {
				t.Fatalf("stack read: %q, %v", got, err)
			}
			for path, original := range before {
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(original, after) {
					t.Fatalf("owned file changed: %s", path)
				}
			}
		})
	}
}
