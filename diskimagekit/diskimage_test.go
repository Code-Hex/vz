package diskimagekit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRAW(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.raw")
	d, err := CreateRAW(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("hello"), 511); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 7)
	if _, err := d.ReadAt(got, 510); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("\x00hello\x00")) {
		t.Fatalf("read %q", got)
	}
	if err := d.Truncate(1); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(8); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadAt(got, 510); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{0, 'h', 0, 0, 0, 0, 0}) {
		t.Fatalf("regrow %q", got)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadAt(got, 0); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed read: %v", err)
	}
	d, err = Open(path, WithOpenMode(ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteAt([]byte("x"), 0); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("readonly write: %v", err)
	}
	if n, err := d.ReadAt(make([]byte, 2), 4095); n != 1 || !errors.Is(err, io.EOF) {
		t.Fatalf("EOF: %d, %v", n, err)
	}
}

func TestASIFStack(t *testing.T) {
	for _, block := range []BlockSize{BlockSize512, BlockSize4096} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.asif")
			d, err := CreateASIF(base, 4<<20/int64(block), WithBlockSize(block))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt(bytes.Repeat([]byte{'B'}, 8192), (1<<20)-4096); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(base)
			if err != nil {
				t.Fatal(err)
			}
			d, err = Open(base)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.CreateLayer(filepath.Join(dir, "cache.asif"), Cache); err != nil {
				t.Fatal(err)
			}
			cacheUUID := d.LayerUUID()
			if err := d.CreateLayer(filepath.Join(dir, "overlay.asif"), Overlay); err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt([]byte{0, 'W', 0}, 1<<20); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 7)
			if _, err := d.ReadAt(got, (1<<20)-2); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte{'B', 'B', 0, 'W', 0, 'B', 'B'}) {
				t.Fatalf("partial COW: %v", got)
			}
			if cacheUUID != d.Layers()[1].LayerUUID {
				t.Fatal("cache UUID changed")
			}
			if err := d.Truncate((1 << 20) / int64(block)); err != nil {
				t.Fatal(err)
			}
			if err := d.Truncate((4 << 20) / int64(block)); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReadAt(got, (1<<20)-2); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte{'B', 'B', 0, 0, 0, 0, 0}) {
				t.Fatalf("regrow: %v", got)
			}
			after, err := os.ReadFile(base)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("parent changed")
			}
		})
	}
}

func TestOpenModeAndUnchangedSize(t *testing.T) {
	for _, format := range []Format{RAW, ASIF} {
		t.Run(fmt.Sprint(format), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "image")
			var d *DiskImage
			var err error
			if format == RAW {
				d, err = CreateRAW(path, 8192)
			} else {
				d, err = CreateASIF(path, 8192)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if d.OpenMode() != ReadWrite {
				t.Errorf("default open mode = %v", d.OpenMode())
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path, WithOpenMode(ReadOnly))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			id := d.LayerUUID()
			if err := d.Truncate(8192); err != nil {
				t.Fatalf("unchanged size on read-only image: %v", err)
			}
			for _, blocks := range []int64{4096, 16384} {
				if err := d.Truncate(blocks); !errors.Is(err, ErrReadOnly) {
					t.Fatalf("truncate to %d = %v", blocks, err)
				}
			}
			if d.LayerUUID() != id || d.BlockCount() != 8192 {
				t.Fatal("read-only truncate changed the image")
			}
		})
	}
}

func TestCreateReplacesPath(t *testing.T) {
	for _, format := range []Format{RAW, ASIF} {
		for _, symlink := range []bool{false, true} {
			t.Run(fmt.Sprintf("format=%d/symlink=%v", format, symlink), func(t *testing.T) {
				dir := t.TempDir()
				original := filepath.Join(dir, "original")
				path := filepath.Join(dir, "image")
				if err := os.WriteFile(original, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				link := os.Link
				if symlink {
					link = os.Symlink
				}
				if err := link(original, path); err != nil {
					t.Fatal(err)
				}
				var d *DiskImage
				var err error
				if format == RAW {
					d, err = CreateRAW(path, 8)
				} else {
					d, err = CreateASIF(path, 8)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer d.Close()
				got := make([]byte, 4096)
				if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, make([]byte, len(got))) {
					t.Fatalf("new image contents: %v", err)
				}
				old, err := os.ReadFile(original)
				if err != nil || string(old) != "keep" {
					t.Fatalf("existing inode changed: %q, %v", old, err)
				}
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 2 {
					t.Fatalf("temporary creation files remain: %v, %v", entries, err)
				}
			})
		}
	}
}

func TestCreateReadOnlyPath(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can replace a read-only file")
	}
	path := filepath.Join(t.TempDir(), "image")
	if err := os.WriteFile(path, []byte("keep"), 0400); err != nil {
		t.Fatal(err)
	}
	for _, format := range []Format{RAW, ASIF} {
		var d *DiskImage
		var err error
		if format == RAW {
			d, err = CreateRAW(path, 8)
		} else {
			d, err = CreateASIF(path, 8)
		}
		if d != nil {
			d.Close()
		}
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("replace read-only file: %v", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "keep" {
		t.Fatalf("read-only file changed: %q, %v", got, err)
	}
}

func TestCreateFollowsDanglingSymlink(t *testing.T) {
	for _, format := range []Format{RAW, ASIF} {
		t.Run(fmt.Sprint(format), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "image")
			if err := os.Symlink("target", path); err != nil {
				t.Fatal(err)
			}
			var image *DiskImage
			var err error
			if format == RAW {
				image, err = CreateRAW(path, 8)
			} else {
				image, err = CreateASIF(path, 8)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer image.Close()
			if target, err := os.Readlink(path); err != nil || target != "target" {
				t.Fatalf("symlink replaced: %q, %v", target, err)
			}
			if info, err := os.Stat(filepath.Join(dir, "target")); err != nil || !info.Mode().IsRegular() {
				t.Fatalf("target not created: %v", err)
			}
		})
	}
}

func TestCreateResolvesSymlinkBeforeParent(t *testing.T) {
	dir := t.TempDir()
	links := filepath.Join(dir, "links")
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(links, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "sub"), filepath.Join(links, "dir")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(links, "image")
	if err := os.Symlink("dir/../target", path); err != nil {
		t.Fatal(err)
	}
	image, err := CreateRAW(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("created image is not reachable through its symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "target")); err != nil {
		t.Fatalf("resolved target was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(links, "target")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created a file in the wrong directory: %v", err)
	}
}
