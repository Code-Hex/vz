//go:build darwin

package vzbridge

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLazyBindingConcurrentRegistration(t *testing.T) {
	var binding lazyBinding[func() int32]
	var loads atomic.Int32
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			call := binding.get(func() func() int32 {
				loads.Add(1)
				return directSymbol[func() int32]("getpid")
			})
			if got := call(); got != int32(os.Getpid()) {
				t.Errorf("getpid = %d, want %d", got, os.Getpid())
			}
		})
	}
	group.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("registrations = %d, want 1", got)
	}
}

func TestLazyBindingRepeatsRegistrationPanic(t *testing.T) {
	var binding lazyBinding[int]
	var first any
	loads := 0
	for range 2 {
		func() {
			defer func() {
				got := recover()
				if got == nil {
					t.Error("invalid registration did not panic")
					return
				}
				if first == nil {
					first = got
				} else if got != first {
					t.Errorf("panic = %v, want %v", got, first)
				}
			}()
			binding.get(func() int {
				loads++
				return directSymbol[int]("getpid")
			})
		}()
	}
	if loads != 1 {
		t.Fatalf("registrations = %d, want 1", loads)
	}
}
