package diskimagekit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type failingStorage struct {
	imageIO
	remaining int
	short     bool
}

func (f *failingStorage) WriteAt(p []byte, off int64) (int, error) {
	f.remaining--
	if f.remaining == 0 {
		if f.short {
			return f.imageIO.WriteAt(p[:len(p)/2], off)
		}
		return 0, syscall.ENOSPC
	}
	return f.imageIO.WriteAt(p, off)
}

func (f *failingStorage) Sync() error {
	f.remaining--
	if f.remaining == 0 {
		return syscall.ENOSPC
	}
	return f.imageIO.Sync()
}

func TestASIFStorageFailure(t *testing.T) {
	// UUID, payload, table, directory body, publication, and their flushes are
	// separate storage operations. Every failure must keep earlier data readable.
	for _, short := range []bool{false, true} {
		for failure := 1; failure <= 8; failure++ {
			if short && (failure == 2 || failure == 6 || failure == 8) {
				continue
			}
			t.Run(fmt.Sprintf("operation=%d/short=%v", failure, short), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "disk.asif")
				d, err := CreateASIF(path, 8192)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := d.WriteAt(bytes.Repeat([]byte{'A'}, 1<<20), 0); err != nil {
					t.Fatal(err)
				}
				if _, err := d.WriteAt([]byte("sentinel"), 2<<20); err != nil {
					t.Fatal(err)
				}
				if err := d.Close(); err != nil {
					t.Fatal(err)
				}
				d, err = Open(path, WithOpenMode(ReadWrite))
				if err != nil {
					t.Fatal(err)
				}
				d.layers[0].io = &failingStorage{
					imageIO:   d.layers[0].file,
					remaining: failure,
					short:     short,
				}
				if _, err := d.WriteAt(bytes.Repeat([]byte{'B'}, 1<<20), 0); err == nil {
					t.Fatal("storage failure was hidden")
				}
				if _, err := d.WriteAt([]byte("retry"), 0); err == nil {
					t.Fatal("mutation allowed after failure")
				}
				if err := d.Sync(); err == nil {
					t.Fatal("Sync hid mutation failure")
				}
				got := make([]byte, 1<<20)
				if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, bytes.Repeat([]byte{'A'}, 1<<20)) {
					t.Fatalf("live prior data changed: %v", err)
				}
				if err := d.Close(); err != nil {
					t.Fatal(err)
				}
				d, err = Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer d.Close()
				if _, err := d.ReadAt(got, 0); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, bytes.Repeat([]byte{'A'}, 1<<20)) && !bytes.Equal(got, bytes.Repeat([]byte{'B'}, 1<<20)) {
					t.Fatal("reopened data mixes generations")
				}
				sentinel := make([]byte, 8)
				if _, err := d.ReadAt(sentinel, 2<<20); err != nil || string(sentinel) != "sentinel" {
					t.Fatalf("unrelated data: %q, %v", sentinel, err)
				}
			})
		}
	}
}

func TestASIFBitmapWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("original"), 0); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d.layers[0].io = &failingStorage{
		imageIO:   d.layers[0].file,
		remaining: 4,
		short:     true,
	}
	if _, err := d.WriteAt([]byte("changed"), 0); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("partial bitmap write: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	got := make([]byte, 8)
	if _, err := d.ReadAt(got, 0); err != nil || string(got) != "original" {
		t.Fatalf("failed bitmap publication changed data: %q, %v", got, err)
	}
}

func TestASIFDirectorySelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.asif")
	d, err := CreateASIF(path, 8192)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("A"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("B"), 0); err != nil {
		t.Fatal(err)
	}
	dirs := d.layers[0].asif.directories
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sequence [8]byte
	bigEndian.PutUint64(sequence[:], 1<<63-1)
	if _, err := f.WriteAt(sequence[:], dirs[0]); err != nil {
		t.Fatal(err)
	}
	bigEndian.PutUint64(sequence[:], 1<<63)
	if _, err := f.WriteAt(sequence[:], dirs[1]); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var got [1]byte
	if _, err := d.ReadAt(got[:], 0); err != nil {
		t.Fatal(err)
	}
	if got[0] != 'A' {
		t.Fatalf("signed directory selection: %q", got[:])
	}
}

func TestASIFShortTailAndInvalidBitmap(t *testing.T) {
	path := compressedFixture(t, "testdata/asif-4096.gz")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	l := d.layers[0]
	e, err := l.extent((1 << 20) + 4096)
	if err != nil {
		t.Fatal(err)
	}
	tail := e.offset + 4096
	a := l.asif
	table, err := l.tableAt(0)
	if err != nil {
		t.Fatal(err)
	}
	bitmap := bigEndian.Uint64(table[a.geometry.entries*8:])
	sector := chunkSize/int64(BlockSize4096) + 1
	bitmapOffset := int64(bitmap)*chunkSize + sector/4
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	// Apple can put its last payload in a short physical chunk. Move that payload
	// behind all metadata, then shorten only the unused physical tail.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 8192)
	if _, err := f.ReadAt(payload, tail-8192); err != nil {
		t.Fatal(err)
	}
	newOffset := (stat.Size() + chunkSize - 1) / chunkSize * chunkSize
	if _, err := f.WriteAt(payload, newOffset); err != nil {
		t.Fatal(err)
	}
	tablePointer := a.directory[0]
	var entry [8]byte
	bigEndian.PutUint64(entry[:], 3<<62|uint64(newOffset/chunkSize))
	if _, err := f.WriteAt(entry[:], int64(tablePointer)*chunkSize+8); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(newOffset + 8192); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4096)
	if _, err := d.ReadAt(got, (1<<20)+4096); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte{'G'}, 4096)) {
		t.Fatal("short-tail payload differs")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	var bitmapByte [1]byte
	if _, err := f.ReadAt(bitmapByte[:], bitmapOffset); err != nil {
		t.Fatal(err)
	}
	bitmapByte[0] |= 3 << ((sector % 4) * 2)
	if _, err := f.WriteAt(bitmapByte[:], bitmapOffset); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ReadAt(got, (1<<20)+4096); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("bitmap state 3: %v", err)
	}
}

type blockedStorage struct {
	imageIO
	entered chan struct{}
	resume  chan struct{}
}

func (b *blockedStorage) ReadAt(p []byte, off int64) (int, error) {
	close(b.entered)
	<-b.resume
	return b.imageIO.ReadAt(p, off)
}

func TestCloseWaitsForRead(t *testing.T) {
	d, err := CreateRAW(filepath.Join(t.TempDir(), "disk.raw"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.WriteAt([]byte("data"), 0); err != nil {
		t.Fatal(err)
	}
	storage := &blockedStorage{
		imageIO: d.layers[0].file,
		entered: make(chan struct{}),
		resume:  make(chan struct{}),
	}
	d.layers[0].io = storage
	readDone := make(chan error, 1)
	go func() {
		got := make([]byte, 4)
		_, err := d.ReadAt(got, 0)
		if err == nil && string(got) != "data" {
			err = fmt.Errorf("read %q", got)
		}
		readDone <- err
	}()
	<-storage.entered
	closeStarted := make(chan struct{})
	closeDone := make(chan error, 1)
	go func() {
		close(closeStarted)
		closeDone <- d.Close()
	}()
	<-closeStarted
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before read finished: %v", err)
	default:
	}
	close(storage.resume)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}

func TestReadReportsPartialData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk.raw")
	d, err := CreateRAW(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.WriteAt([]byte("abc"), 0); err != nil {
		t.Fatal(err)
	}
	// A writer that ignores advisory locks can shorten the backing file.
	if err := os.Truncate(path, 3); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 8)
	n, err := d.ReadAt(got, 0)
	if n != 3 || !errors.Is(err, io.EOF) || string(got[:n]) != "abc" {
		t.Fatalf("read %d %q, %v", n, got, err)
	}
}

func TestASIFPayloadEOF(t *testing.T) {
	for _, blocks := range []int64{1, 2048} {
		t.Run(fmt.Sprint(blocks), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "disk.asif")
			d, err := CreateASIF(path, blocks)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := d.WriteAt(bytes.Repeat([]byte{'A'}, 512), 0); err != nil {
				t.Fatal(err)
			}
			a := d.layers[0].asif
			table := int64(a.directory[0]) * chunkSize
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
			tail := (stat.Size() + chunkSize - 1) / chunkSize * chunkSize
			if _, err := f.WriteAt(bytes.Repeat([]byte{'A'}, 512), tail); err != nil {
				t.Fatal(err)
			}
			var entry [8]byte
			bigEndian.PutUint64(entry[:], 1<<62|uint64(tail/chunkSize))
			if _, err := f.WriteAt(entry[:], table); err != nil {
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
			got := bytes.Repeat([]byte{0xa5}, int(blocks)*512)
			if _, err := d.ReadAt(got, 0); err != nil {
				t.Fatal(err)
			}
			want := make([]byte, len(got))
			copy(want, bytes.Repeat([]byte{'A'}, 512))
			if !bytes.Equal(got, want) {
				t.Fatal("payload EOF did not read as zero")
			}
			n, err := d.WriteAt([]byte{'X'}, 0)
			if n != 1 || err != nil {
				t.Fatalf("tail write: %d, %v", n, err)
			}
			if err := d.Close(); err != nil {
				t.Fatal(err)
			}
			d, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			want[0] = 'X'
			if _, err := d.ReadAt(got, 0); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("updated tail: %v", err)
			}
		})
	}
}
