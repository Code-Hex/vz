package diskimagekit

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func compressedFixture(t *testing.T, path string) string {
	t.Helper()
	source, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	compressed, err := gzip.NewReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	path = filepath.Join(t.TempDir(), "image.asif")
	destination, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(destination, compressed); err != nil {
		destination.Close()
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// These fixtures were created by Apple's DiskImageKit and written by a Linux
// guest through Virtualization.framework on macOS 27.0.1. They include allocated
// zero, nonzero data, discarded sectors, and unallocated chunks.
func TestReadAppleASIF(t *testing.T) {
	for _, block := range []BlockSize{BlockSize512, BlockSize4096} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			path := compressedFixture(t, fmt.Sprintf("testdata/asif-%d.gz", block))
			d, err := Open(path, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			want := make([]byte, 4<<20)
			copy(want[(1<<20)+4096:], bytes.Repeat([]byte{'G'}, 4096))
			got := make([]byte, len(want))
			if _, err := d.ReadAt(got, 0); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("Apple data differs")
			}
			if _, err := d.WriteAt([]byte("Go"), (1<<20)+8191); err != nil {
				t.Fatal(err)
			}
			copy(want[(1<<20)+8191:], []byte("Go"))
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if _, err := reopened.ReadAt(got, 0); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("updated Apple image differs")
			}
		})
	}
}

func TestOverlayGrowth(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.raw")
	d, err := CreateRAW(base, 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteAt(bytes.Repeat([]byte{'P'}, 1<<20), 0); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateLayer(filepath.Join(dir, "overlay.asif"), Overlay, WithBlockCount(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("X"), 0); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(4096); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 1024)
	if _, err := d.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	want := bytes.Repeat([]byte{'P'}, 1024)
	want[0] = 'X'
	if !bytes.Equal(got, want) {
		t.Fatal("growth lost parent data")
	}
	if err := d.Truncate(1); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(4096); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	clear(want[512:])
	if !bytes.Equal(got, want) {
		t.Fatal("regrowth exposed discarded parent data")
	}
}

func TestASIFTableBoundaries(t *testing.T) {
	for _, block := range []BlockSize{BlockSize512, BlockSize4096} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "large.asif")
			d, err := CreateASIF(path, (130<<30)/int64(block), WithBlockSize(block))
			if err != nil {
				t.Fatal(err)
			}
			offsets := []int64{(2 << 30) - 3, (16 << 30) - 3, (112 << 30) - 3, (126 << 30) - 3}
			for _, off := range offsets {
				if _, err := d.WriteAt([]byte("boundary"), off); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			for _, off := range offsets {
				got := make([]byte, 10)
				if _, err := d.ReadAt(got, off-1); err != nil {
					t.Fatal(err)
				}
				if string(got) != "\x00boundary\x00" {
					t.Fatalf("offset %d: %q", off, got)
				}
			}
		})
	}
}

func TestStackValidationAndCache(t *testing.T) {
	dir := t.TempDir()
	d, err := CreateASIF(filepath.Join(dir, "base.asif"), 8192)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteAt([]byte("parent"), 0); err != nil {
		t.Fatal(err)
	}
	baseID := d.LayerUUID()
	cache := filepath.Join(dir, "cache.asif")
	if err := d.CreateLayer(cache, Cache); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("bad"), 0); err == nil {
		t.Fatal("cache write accepted")
	}
	if err := d.Truncate(1024); err == nil {
		t.Fatal("cache truncate accepted")
	}
	if err := d.CreateLayer(filepath.Join(dir, "extra.asif"), Cache); !errors.Is(err, ErrIncompatibleStack) {
		t.Fatalf("second cache: %v", err)
	}
	got := make([]byte, 6)
	if _, err := d.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	if string(got) != "parent" || d.LayerUUID() != baseID {
		t.Fatalf("cache %q, UUID %q", got, d.LayerUUID())
	}
	cachedSize, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	rereadSize, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}
	if cachedSize.Size() != rereadSize.Size() {
		t.Fatal("cache hit allocated storage")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	c, err := Open(cache, WithOpenMode(ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	if c.OpenMode() != ReadWrite || c.Layers()[0].LayerType != 0 {
		t.Error("standalone cache has a stack role or logical read-only mode")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(cache, WithOpenMode(ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ReadAt(got, 0); err != nil || string(got) != "parent" {
		t.Fatalf("saved cache: %q, %v", got, err)
	}
	if _, err := c.ReadAt(got, 1<<20); !errors.Is(err, ErrIncompatibleStack) {
		t.Fatalf("readonly cache miss: %v", err)
	}
	base, err := Open(filepath.Join(dir, "base.asif"))
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	if err := base.Append(cache, WithOpenMode(ReadOnly)); err != nil {
		t.Fatalf("read-only cache append: %v", err)
	}
	if _, err := base.ReadAt(got, 0); err != nil || string(got) != "parent" {
		t.Fatalf("read-only cache hit: %q, %v", got, err)
	}
	if _, err := base.ReadAt(got, 1<<20); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only cache miss: %v", err)
	}
	orphan, err := CreateASIF(filepath.Join(dir, "orphan.asif"), 8192)
	if err != nil {
		t.Fatal(err)
	}
	if err := orphan.Close(); err != nil {
		t.Fatal(err)
	}
	if err := base.Append(filepath.Join(dir, "orphan.asif")); !errors.Is(err, ErrIncompatibleStack) {
		t.Fatalf("UUID mismatch: %v", err)
	}
	if _, err := base.ReadAt(got, 0); err != nil || string(got) != "parent" {
		t.Fatalf("failed append changed stack: %q, %v", got, err)
	}
}

func TestInvalidInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "disk.asif")
	for _, blocks := range []int64{-1, 0, 1<<63 - 1} {
		if d, err := CreateASIF(path, blocks); err == nil {
			d.Close()
			t.Fatalf("accepted size %d", blocks)
		}
	}
	if d, err := CreateASIF(path, 1, WithBlockSize(1024)); err == nil {
		d.Close()
		t.Fatal("accepted block size")
	}
	if d, err := Open(path, WithOpenMode(255)); err == nil {
		d.Close()
		t.Fatal("accepted open mode")
	}
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name     string
		mutate   func([]byte)
		deferred bool
	}{
		{"version", func(b []byte) { bigEndian.PutUint32(b[4:], 2) }, false},
		{"block", func(b []byte) { bigEndian.PutUint16(b[0x44:], 1024) }, false},
		{"chunk", func(b []byte) { bigEndian.PutUint32(b[0x40:], 4096) }, true},
		{"size", func(b []byte) { bigEndian.PutUint64(b[0x30:], ^uint64(0)) }, false},
		{"metadata overlap", func(b []byte) { bigEndian.PutUint64(b[0x48:], 1) }, true},
		{"directory overlap", func(b []byte) { copy(b[0x18:0x20], b[0x10:0x18]) }, true},
		{"directory pointer", func(b []byte) { off := bigEndian.Uint64(b[0x18:]); bigEndian.PutUint64(b[off+8:], ^uint64(0)) }, true},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			data := append([]byte(nil), original...)
			test.mutate(data)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := Open(path)
			if test.deferred {
				if err != nil {
					t.Fatalf("metadata-only open: %v", err)
				}
				defer got.Close()
				if got.Format() != ASIF || got.BlockCount() != 8192 {
					t.Fatal("header properties changed")
				}
				_, err = got.ReadAt(make([]byte, 512), 0)
			}
			if !errors.Is(err, ErrInvalidImage) {
				if got != nil {
					got.Close()
				}
				t.Fatalf("invalid input: %v", err)
			}
		})
	}
}

func TestLocksAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.raw")
	d, err := CreateRAW(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	alias := path + ".link"
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []OpenMode{ReadOnly, ReadWrite, Automatic} {
		if other, err := Open(alias, WithOpenMode(mode)); err == nil {
			other.Close()
			t.Fatalf("lock bypass with mode %d", mode)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path, WithOpenMode(ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	other, err := Open(alias, WithOpenMode(ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var workers sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for j := 0; j < 20; j++ {
				b := make([]byte, 512)
				if _, err := d.ReadAt(b, 0); err != nil && !errors.Is(err, os.ErrClosed) {
					t.Errorf("concurrent read: %v", err)
					return
				}
			}
		}()
	}
	close(start)
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if err := d.Sync(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("sync after close: %v", err)
	}
	if err := d.Truncate(4); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("truncate after close: %v", err)
	}
	if _, err := d.WriteAt([]byte("x"), 0); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
}

func TestMissingParentDoesNotReplaceInheritedData(t *testing.T) {
	for _, kind := range []LayerType{Cache, Overlay} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.asif")
			child := filepath.Join(dir, "child.asif")
			d, err := CreateASIF(base, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt([]byte("parent"), 0); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(child, kind); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(child)
			if err != nil {
				t.Fatal(err)
			}
			detached, err := Open(child, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 6)
			if _, err := detached.ReadAt(got, 0); !errors.Is(err, ErrIncompatibleStack) {
				t.Fatalf("missing parent read: %v", err)
			}
			if kind == Overlay {
				if _, err := detached.WriteAt([]byte("x"), 0); !errors.Is(err, ErrIncompatibleStack) {
					t.Fatalf("missing parent write: %v", err)
				}
			}
			if err := detached.Close(); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(child)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("missing parent changed child")
			}
			d, err = Open(base)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.Append(child, WithOpenMode(ReadWrite)); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReadAt(got, 0); err != nil || string(got) != "parent" {
				t.Fatalf("reattached child: %q, %v", got, err)
			}
		})
	}
}

func TestASIFCapacityLimit(t *testing.T) {
	const size = int64(4<<50 - 1<<20)
	for _, block := range []BlockSize{BlockSize512, BlockSize4096} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image.asif")
			blocks := size / int64(block)
			d, err := CreateASIF(path, blocks, WithBlockSize(block))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			want := make([]byte, int(block))
			copy(want[len(want)-4:], "edge")
			if _, err := d.WriteAt(want, size-int64(block)); err != nil {
				t.Fatal(err)
			}
			if err := d.Sync(); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			d, err = Open(path, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			got := make([]byte, len(want))
			if _, err := d.ReadAt(got, size-int64(block)); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("last block after reopening: %v, %v", got, err)
			}
			if err := d.Truncate(blocks + 1); err == nil {
				t.Fatal("accepted size overlapping metadata")
			}
			if d.Size() != size {
				t.Fatalf("failed truncate changed size to %d", d.Size())
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("failed truncate changed image")
			}
			if image, err := CreateASIF(path, blocks+1, WithBlockSize(block)); err == nil {
				image.Close()
				t.Fatal("created image overlapping metadata")
			}
			after, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("failed creation changed existing image")
			}
		})
	}
}
