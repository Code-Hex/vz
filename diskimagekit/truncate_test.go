package diskimagekit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestASIFTruncateMappings(t *testing.T) {
	for _, test := range []struct {
		name       string
		initial    int64
		shrunk     int64
		grown      int64
		otherChunk bool
		ownBytes   int
		ownOffset  int64
		want       []byte
	}{
		{"missing table", 2049, 2048, 2050, false, 0, 0, []byte{'B', 'B'}},
		{"inherited short tail", 2049, 2048, 2050, true, 0, 0, []byte{'B', 'B'}},
		{"inherited collapse", 2050, 2049, 2051, false, 512, 512, []byte{'B', 'B', 'B'}},
		{"retained sparse tail", 2050, 2049, 2051, false, 1024, 0, []byte{'W', 0, 'B'}},
		{"inherited full chunk", 4096, 2048, 4096, true, 0, 0, []byte{0, 0}},
		{"inherited partial chunk", 4096, 2049, 4096, true, 0, 0, []byte{'B', 'B'}},
		{"sparse partial chunk", 4096, 2049, 4096, false, 1024, 0, []byte{'W', 0}},
		{"sparse short tail", 2049, 2048, 2050, false, 512, 0, []byte{0, 0}},
		{"allocated partial chunk", 4096, 2049, 4096, false, 1 << 20, 0, []byte{'W', 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.raw")
			overlay := filepath.Join(dir, "overlay.asif")
			d, err := CreateRAW(base, 8192)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { d.Close() })
			if _, err := d.WriteAt(bytes.Repeat([]byte{'B'}, 4<<20), 0); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(overlay, Overlay, WithBlockCount(test.initial)); err != nil {
				t.Fatal(err)
			}
			if test.otherChunk {
				if _, err := d.WriteAt([]byte{'S'}, 0); err != nil {
					t.Fatal(err)
				}
			}
			if test.ownBytes != 0 {
				if _, err := d.WriteAt(bytes.Repeat([]byte{'W'}, test.ownBytes), (1<<20)+test.ownOffset); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.Truncate(test.shrunk); err != nil {
				t.Fatal(err)
			}
			if err := d.Truncate(test.grown); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(base, WithOpenMode(ReadOnly))
			if err != nil {
				t.Fatal(err)
			}
			if err := d.Append(overlay, WithOpenMode(ReadOnly)); err != nil {
				t.Fatal(err)
			}
			for i, value := range test.want {
				got := make([]byte, 512)
				if _, err := d.ReadAt(got, (1<<20)+int64(i)*512); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, bytes.Repeat([]byte{value}, len(got))) {
					t.Fatalf("sector %d after regrowth: got %d, want %d", i, got[0], value)
				}
			}
		})
	}
}

func TestASIFTruncateSkipsMissingTables(t *testing.T) {
	const size = int64(128 << 30)
	const tableBoundary = int64(126 << 30)
	dir := t.TempDir()
	base := filepath.Join(dir, "base.asif")
	overlay := filepath.Join(dir, "overlay.asif")
	d, err := CreateASIF(base, size/512)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	for _, off := range []int64{1 << 20, tableBoundary} {
		if _, err := d.WriteAt(bytes.Repeat([]byte{'B'}, 512), off); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.CreateLayer(overlay, Overlay); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte{'S'}, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate((1 << 20) / 512); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(size / 512); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(base, WithOpenMode(ReadOnly))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Append(overlay, WithOpenMode(ReadOnly)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		off  int64
		want byte
	}{{1 << 20, 0}, {tableBoundary, 'B'}} {
		got := make([]byte, 512)
		if _, err := d.ReadAt(got, test.off); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, bytes.Repeat([]byte{test.want}, len(got))) {
			t.Fatalf("offset %d after regrowth: got %d, want %d", test.off, got[0], test.want)
		}
	}
}

func TestASIFTruncateSparseCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sparse.asif")
	d, err := CreateASIF(path, (1<<40)/512)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(1); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() {
		t.Fatalf("shrinking an untouched image allocated storage: %d to %d", before.Size(), after.Size())
	}
}
