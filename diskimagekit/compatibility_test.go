package diskimagekit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestASIFHeaderWithoutDirectories(t *testing.T) {
	for _, capacity := range []uint64{0, uint64(math.MaxInt64)/512 + 1} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "header.asif")
			d, err := CreateASIF(path, 8192)
			if err != nil {
				t.Fatal(err)
			}
			d.Close()
			header, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			blocks := min(capacity, 8192)
			bigEndian.PutUint64(header[0x30:], blocks)
			bigEndian.PutUint64(header[0x38:], capacity)
			if err := os.WriteFile(path, header[:512], 0600); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if d.Format() != ASIF || d.BlockCount() != int64(blocks) {
				t.Fatal("header metadata differs")
			}
			for i := 0; i < 2; i++ {
				_, err := d.ReadAt(make([]byte, 512), 0)
				want := ErrInvalidImage
				if blocks == 0 {
					want = io.EOF
				}
				if !errors.Is(err, want) {
					t.Fatalf("read without directories = %v", err)
				}
			}
		})
	}
}

func TestSmallCacheModeAndBounds(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.raw")
	cache := filepath.Join(dir, "cache.asif")
	d, err := CreateRAW(base, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.CreateLayer(cache, Cache); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(base, WithOpenMode(ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(16384); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("parent"), 6<<20); err != nil {
		t.Fatal(err)
	}
	if err := d.Append(cache, WithOpenMode(ReadWrite)); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.OpenMode() != ReadOnly || d.Layers()[1].OpenMode != ReadWrite {
		t.Fatal("cache mode does not distinguish image and file access")
	}
	if d.Size() != 4<<20 {
		t.Fatalf("cache size %d", d.Size())
	}
	if err := d.CreateLayer(filepath.Join(dir, "overlay.asif"), Overlay, WithBlockCount(16384)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 6)
	if _, err := d.ReadAt(got, 6<<20); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, make([]byte, 6)) {
		t.Fatalf("inherited beyond smaller cache: %q", got)
	}
}

func TestASIFHeaderFlags(t *testing.T) {
	cases := []struct {
		name     string
		offset   int64
		value    []byte
		invalid  bool
		raw      bool
		readonly bool
	}{
		{
			name:     "data readonly",
			offset:   0x60,
			value:    []byte{0, 0, 0, 1},
			readonly: true,
		},
		{
			name:     "unknown data readonly",
			offset:   0x60,
			value:    []byte{128, 0, 0, 0},
			readonly: true,
		},
		{
			name:   "metadata readonly",
			offset: 0x68,
			value:  []byte{0, 0, 0, 1},
		},
		{
			name:   "metadata flags",
			offset: 0x64,
			value:  []byte{0, 0, 0, 1},
			raw:    true,
		},
		{
			name:   "segment",
			offset: 0x46,
			value:  []byte{0, 1},
			raw:    true,
		},
		{
			name:   "unknown flags",
			offset: 12,
			value:  []byte{0, 0, 0, 2},
			raw:    true,
		},
		{
			name:    "reserved",
			offset:  0x6c,
			value:   []byte{1},
			invalid: true,
		},
		{
			name:   "metadata absent",
			offset: 0x48,
			value:  make([]byte, 8),
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "disk.asif")
			d, err := CreateASIF(path, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteAt(test.value, test.offset); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			d, err = Open(path, WithOpenMode(Automatic))
			if test.invalid {
				if !errors.Is(err, ErrInvalidImage) {
					if d != nil {
						d.Close()
					}
					t.Fatalf("invalid flags: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if test.raw {
				if d.Format() != RAW || d.Size() != int64(len(before)) {
					t.Fatalf("format detection = %v, size %d", d.Format(), d.Size())
				}
				got := make([]byte, len(before))
				if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, before) {
					t.Fatalf("RAW fallback changed content: %v", err)
				}
				return
			}
			_, err = d.WriteAt([]byte("value"), 0)
			if test.readonly {
				if !errors.Is(err, ErrReadOnly) {
					t.Fatalf("readonly write: %v", err)
				}
				if err := d.Truncate(4096); !errors.Is(err, ErrReadOnly) {
					t.Fatalf("readonly truncate: %v", err)
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("readonly image changed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := make([]byte, 5)
			if _, err := d.ReadAt(got, 0); err != nil || string(got) != "value" {
				t.Fatalf("write/read %q, %v", got, err)
			}
		})
	}
}

func TestASIFGeometry(t *testing.T) {
	for _, test := range []struct {
		block BlockSize
		chunk int64
	}{
		{BlockSize512, 65536},
		{BlockSize512, 98304},
		{BlockSize512, 2 << 20},
		{BlockSize4096, 262144},
		{BlockSize4096, 393216},
		{BlockSize4096, 2 << 20},
	} {
		t.Run(fmt.Sprintf("%d/%d", test.block, test.chunk), func(t *testing.T) {
			path := compressedFixture(t, fmt.Sprintf("testdata/geometry-%d-%d.gz", test.block, test.chunk))
			d, err := Open(path, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			got := make([]byte, 512)
			if _, err := d.ReadAt(got, test.chunk); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, bytes.Repeat([]byte{'A'}, 512)) {
				t.Fatal("reference payload differs")
			}
			if _, err := d.WriteAt([]byte("Go"), test.chunk+1); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if _, err := d.ReadAt(got, test.chunk); err != nil {
				t.Fatal(err)
			}
			want := bytes.Repeat([]byte{'A'}, 512)
			copy(want[1:], "Go")
			if !bytes.Equal(got, want) {
				t.Fatal("updated geometry payload differs")
			}
		})
	}
}

func TestASIFReusesUnreferencedChunks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("sentinel"), 2<<20); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		d, err = Open(path, WithOpenMode(ReadWrite))
		if err != nil {
			t.Fatal(err)
		}
		want := bytes.Repeat([]byte{byte(i)}, 1<<20)
		if _, err := d.WriteAt(want, 0); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(want))
		if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("iteration %d data: %v", i, err)
		}
		sentinel := make([]byte, 8)
		if _, err := d.ReadAt(sentinel, 2<<20); err != nil || string(sentinel) != "sentinel" {
			t.Fatalf("iteration %d sentinel: %q %v", i, sentinel, err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() > 12<<20 {
		t.Fatalf("repeated writes grew to %d", stat.Size())
	}
}

func TestASIFUnpublishedDirectories(t *testing.T) {
	for _, which := range []int{-1, 0, 1} {
		t.Run(fmt.Sprint(which), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "disk.asif")
			d, err := CreateASIF(path, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if which == -1 || which == i {
					if _, err := f.WriteAt(make([]byte, 8), 0x10+int64(i)*8); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt([]byte("published"), 0); err != nil {
				t.Fatal(err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			got := make([]byte, 9)
			if _, err := d.ReadAt(got, 0); err != nil || string(got) != "published" {
				t.Fatalf("reopened root: %q %v", got, err)
			}
		})
	}
}

func TestASIFShortTable(t *testing.T) {
	for _, length := range []int{0, 512} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "disk.asif")
			d, err := CreateASIF(path, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReadAt(make([]byte, 1), 0); err != nil {
				t.Fatal(err)
			}
			a := d.layers[0].asif
			dir := a.directories[a.selected]
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			stat, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			var pointer [8]byte
			bigEndian.PutUint64(pointer[:], uint64(stat.Size()/chunkSize))
			if _, err := f.WriteAt(pointer[:], dir+8); err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteAt(make([]byte, length), stat.Size()); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			got := make([]byte, 512)
			if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, make([]byte, 512)) {
				t.Fatalf("short table read: %v", err)
			}
			if _, err := d.WriteAt([]byte("data"), 0); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReadAt(got[:4], 0); err != nil || string(got[:4]) != "data" {
				t.Fatalf("short table write: %q, %v", got[:4], err)
			}
		})
	}
}

func TestASIFCapacityBeyondFourPiB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	capacity := int64(8 << 50)
	geometry := newGeometry(BlockSize512, uint64(capacity), chunkSize)
	first, second := int64(512), int64(512)+geometry.directoryBytes
	clear(image[first : second+geometry.directoryBytes])
	bigEndian.PutUint64(image[0x10:], uint64(first))
	bigEndian.PutUint64(image[0x18:], uint64(second))
	bigEndian.PutUint64(image[0x38:], uint64(capacity/512))
	bigEndian.PutUint64(image[0x48:], 0)
	bigEndian.PutUint64(image[second:], 1)
	if err := os.WriteFile(path, image, 0600); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path, WithOpenMode(ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Truncate((capacity - chunkSize) / 512); err != nil {
		t.Fatal(err)
	}
	off := capacity - chunkSize - 1
	if _, err := d.WriteAt([]byte{'X'}, off); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	got := make([]byte, 2)
	if _, err := d.ReadAt(got, off-1); err != nil || !bytes.Equal(got, []byte{0, 'X'}) {
		t.Fatalf("large capacity: %v, %v", got, err)
	}
}

func TestASIFRepairsUnusableInactiveDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("old"), 0); err != nil {
		t.Fatal(err)
	}
	inactive := d.layers[0].asif.directories[1-d.layers[0].asif.selected]
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var invalidPointer [8]byte
	bigEndian.PutUint64(invalidPointer[:], ^uint64(0))
	if _, err := f.WriteAt(invalidPointer[:], inactive+8); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		d, err = Open(path, WithOpenMode(ReadWrite))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.WriteAt([]byte("new"), 1<<20); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 3)
		if _, err := d.ReadAt(got, 0); err != nil || string(got) != "old" {
			t.Fatalf("old data: %q %v", got, err)
		}
		if _, err := d.ReadAt(got, 1<<20); err != nil || string(got) != "new" {
			t.Fatalf("new data: %q %v", got, err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() > 14<<20 {
		t.Fatalf("repaired root did not permit reuse: %d", stat.Size())
	}
}

func TestASIFFinalChunkAtMaximumOffset(t *testing.T) {
	const size = int64(1<<63-1) / 512 * 512
	const chunk = int64(8<<20) - 512
	g := newGeometry(BlockSize512, uint64(size), chunk)
	first, second := int64(512), int64(512)+g.directoryBytes
	path := filepath.Join(t.TempDir(), "maximum.asif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(3 * chunk); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 512)
	copy(header, "shdw")
	bigEndian.PutUint32(header[4:], 1)
	bigEndian.PutUint32(header[8:], 512)
	bigEndian.PutUint64(header[16:], uint64(first))
	bigEndian.PutUint64(header[24:], uint64(second))
	header[32] = 1
	bigEndian.PutUint64(header[48:], uint64(size/512))
	bigEndian.PutUint64(header[56:], uint64(size/512))
	bigEndian.PutUint32(header[64:], uint32(chunk))
	bigEndian.PutUint16(header[68:], 512)
	if _, err := f.WriteAt(header, 0); err != nil {
		t.Fatal(err)
	}
	logical := (size - 1) / chunk * chunk
	var pointer [8]byte
	bigEndian.PutUint64(pointer[:], 1)
	for _, dir := range []int64{first, second} {
		if _, err := f.WriteAt(pointer[:], dir+8*(logical/g.covered+1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.WriteAt(pointer[:], second); err != nil {
		t.Fatal(err)
	}
	table := make([]byte, chunk)
	relative := logical % g.covered
	group := relative / (g.entries * chunk) * g.groupBytes
	index := (relative / chunk) % g.entries
	bigEndian.PutUint64(table[group+index*8:], 3<<62)
	bigEndian.PutUint64(table[group+g.entries*8:], 2)
	if _, err := f.WriteAt(table, chunk); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(bytes.Repeat([]byte{0xaa}, int(chunk)), 2*chunk); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path, WithOpenMode(ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte{'X'}, size-1); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(size/512 - 1); err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(size / 512); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 512)
	if _, err := d.ReadAt(got, size-512); err != nil || !bytes.Equal(got, make([]byte, 512)) {
		t.Fatalf("maximum offset regrowth: %v", err)
	}
	if _, err := d.WriteAt([]byte{'X'}, size-1); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	got = make([]byte, 2)
	if _, err := d.ReadAt(got, size-2); err != nil || !bytes.Equal(got, []byte{0, 'X'}) {
		t.Fatalf("maximum offset read: %v %v", got, err)
	}
}

func TestASIFAllocationPreservesMetadataChain(t *testing.T) {
	path := compressedFixture(t, "testdata/metadata-chain.gz")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const chunk = 65536
	for i := 0; i < 20; i++ {
		d, err := Open(path, WithOpenMode(ReadWrite))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.WriteAt([]byte{byte(i)}, chunk); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 2)
		if _, err := d.ReadAt(got, chunk); err != nil || !bytes.Equal(got, []byte{byte(i), 'A'}) {
			t.Fatalf("payload %v, %v", got, err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, physical := range []int{2, 5} {
		if !bytes.Equal(before[physical*chunk:(physical+1)*chunk], after[physical*chunk:(physical+1)*chunk]) {
			t.Fatalf("metadata chunk %d changed", physical)
		}
	}
	if len(after) > 12*chunk {
		t.Fatalf("metadata chain image grew to %d", len(after))
	}
}

func TestASIFMetadataChainCapacity(t *testing.T) {
	const chunk = int64(65536)
	const limit = 4094 * chunk
	for _, descending := range []bool{false, true} {
		t.Run(fmt.Sprint(descending), func(t *testing.T) {
			path := compressedFixture(t, "testdata/metadata-chain.gz")
			image, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if descending {
				bigEndian.PutUint64(image[0x48:], 4095)
				group := chunk + 8*(2048+1)
				bigEndian.PutUint64(image[group+2047*8:], 3<<62|2)
				bigEndian.PutUint64(image[group+2046*8:], 3<<62|5)
				bigEndian.PutUint64(image[2*chunk+20:], 4094)
			}
			if err := os.WriteFile(path, image, 0600); err != nil {
				t.Fatal(err)
			}
			d, err := Open(path, WithOpenMode(ReadWrite))
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.Truncate(limit / 512); err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt([]byte("edge"), limit-4); err != nil {
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
			got := make([]byte, 4)
			if _, err := d.ReadAt(got, limit-4); err != nil || string(got) != "edge" {
				t.Fatalf("last safe bytes: %q, %v", got, err)
			}
			if err := d.Truncate(limit/512 + 1); err == nil {
				t.Fatal("accepted size overlapping metadata chain")
			}
			if d.Size() != limit {
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
			bigEndian.PutUint64(after[0x30:], uint64(limit/512+1))
			if err := os.WriteFile(path, after, 0600); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if _, err := d.ReadAt(got, 0); !errors.Is(err, ErrInvalidImage) {
				t.Fatalf("overlapping image read: %v", err)
			}
		})
	}
}

func TestASIFChunkStates(t *testing.T) {
	for status := uint64(0); status < 4; status++ {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "base.asif")
			overlay := filepath.Join(dir, "overlay.asif")
			d, err := CreateASIF(base, 8192)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt(bytes.Repeat([]byte{'P'}, 512), 0); err != nil {
				t.Fatal(err)
			}
			if err := d.CreateLayer(overlay, Overlay); err != nil {
				t.Fatal(err)
			}
			if _, err := d.ReadAt(make([]byte, 1), 0); err != nil {
				t.Fatal(err)
			}
			directories := d.layers[1].asif.directories
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(overlay, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			table := make([]byte, chunkSize)
			physical := uint64(0)
			if status == 0 || status == 2 {
				physical = 5
			}
			bigEndian.PutUint64(table, status<<62|physical)
			if status == 3 {
				bigEndian.PutUint64(table[2048*8:], 5)
				if _, err := f.WriteAt(bytes.Repeat([]byte{0x55}, int(chunkSize)), 5*chunkSize); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.WriteAt(table, 4*chunkSize); err != nil {
				t.Fatal(err)
			}
			var pointer [8]byte
			bigEndian.PutUint64(pointer[:], 4)
			for _, offset := range directories {
				if _, err := f.WriteAt(pointer[:], offset+8); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(base)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if err := d.Append(overlay, WithOpenMode(ReadWrite)); err != nil {
				t.Fatal(err)
			}
			got := bytes.Repeat([]byte{0xa5}, 512)
			want := make([]byte, 512)
			if status == 0 {
				copy(want, bytes.Repeat([]byte{'P'}, 512))
			}
			if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("chunk state read: %v", err)
			}
			if _, err := d.WriteAt([]byte{'X'}, 0); err != nil {
				t.Fatal(err)
			}
			want[0] = 'X'
			if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("chunk state write: %v", err)
			}
		})
	}
}

func TestASIFRelocatedDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relocated.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt(bytes.Repeat([]byte{'A'}, 512), 0); err != nil {
		t.Fatal(err)
	}
	original := d.layers[0].asif.directories
	length := d.layers[0].asif.geometry.directoryBytes
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	relocated := [2]int64{8 * chunkSize, 8*chunkSize + length}
	image := make([]byte, relocated[1]+length)
	copy(image, before)
	for i, offset := range relocated {
		copy(image[offset:offset+length], before[original[i]:original[i]+length])
		bigEndian.PutUint64(image[0x10+i*8:], uint64(offset))
	}
	if err := os.WriteFile(path, image, 0600); err != nil {
		t.Fatal(err)
	}
	var settledSize int64
	for i := 0; i < 8; i++ {
		d, err = Open(path, WithOpenMode(ReadWrite))
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Truncate(4096); err != nil {
			t.Fatal(err)
		}
		if _, err := d.WriteAt([]byte{byte(i)}, 0); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 512)
		want := bytes.Repeat([]byte{'A'}, 512)
		want[0] = byte(i)
		if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("relocated directory data: %v", err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		stat, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			settledSize = stat.Size()
		} else if i > 3 && stat.Size() != settledSize {
			t.Fatalf("repeated writes keep growing the file: %d to %d", settledSize, stat.Size())
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before[2*chunkSize:4*chunkSize], after[2*chunkSize:4*chunkSize]) {
		t.Fatal("metadata below directory changed")
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadAt(make([]byte, 1), 0); err != nil {
		t.Fatal(err)
	}
	active := d.layers[0].asif.directories[d.layers[0].asif.selected]
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	bigEndian.PutUint64(after[active+8:], uint64(relocated[0]/chunkSize))
	if err := os.WriteFile(path, after, 0600); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path, WithOpenMode(ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteAt([]byte("bad"), 0); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("directory overlap: %v", err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, unchanged) {
		t.Fatal("overlap rejection modified file")
	}
}

func TestOpenASIFWithZeroBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(make([]byte, 8), 0x30); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Size() != 0 || d.Format() != ASIF {
		t.Fatalf("zero-size image: %d, %d", d.Size(), d.Format())
	}
	if n, err := d.ReadAt(make([]byte, 1), 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("zero-size read: %d %v", n, err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path, WithOpenMode(ReadWrite))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Truncate(1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("grown"), 0); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	got := make([]byte, 5)
	if _, err := d.ReadAt(got, 0); err != nil || string(got) != "grown" {
		t.Fatalf("grown data: %q %v", got, err)
	}
}
