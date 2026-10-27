//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package diskimagekit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestOpenRejectsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		image, err := Open(path, WithOpenMode(ReadOnly))
		if image != nil {
			image.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Open accepted a FIFO")
		}
	case <-time.After(time.Second):
		f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		<-done
		t.Fatal("Open waited for a FIFO writer")
	}
}

func TestCreateUsesDefaultFilePermissions(t *testing.T) {
	dir := t.TempDir()
	control := filepath.Join(dir, "control")
	if err := os.WriteFile(control, nil, 0666); err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "image")
	image, err := CreateRAW(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	got, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("permissions = %o, want %o", got.Mode().Perm(), want.Mode().Perm())
	}
}

func TestCreateFailurePreservesPath(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "image")
			if existing {
				if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			f, err := createFile(path, func(f *os.File) error {
				if _, err := f.Write([]byte("partial")); err != nil {
					return err
				}
				return syscall.ENOSPC
			})
			if f != nil {
				f.Close()
				t.Fatal("failed creation returned a file")
			}
			if !errors.Is(err, syscall.ENOSPC) {
				t.Fatalf("creation error = %v", err)
			}
			data, err := os.ReadFile(path)
			if existing {
				if err != nil || string(data) != "original" {
					t.Fatalf("original changed: %q, %v", data, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed creation left a file: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if existing {
				want = 1
			}
			if len(entries) != want {
				t.Fatalf("temporary files remain: %v", entries)
			}
		})
	}
}
