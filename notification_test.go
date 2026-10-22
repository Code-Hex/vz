//go:build darwin

package vz

import (
	"sync"
	"testing"
)

func TestNotifications(t *testing.T) {
	var events notifications[int]
	buffered := make(chan int, 2)
	unbuffered := make(chan int)
	other := make(chan int, 3)
	stop, err := events.subscribe(buffered)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := events.subscribe(buffered)
	if err != nil {
		t.Fatal(err)
	}
	stopUnbuffered, err := events.subscribe(unbuffered)
	if err != nil {
		t.Fatal(err)
	}
	defer stopUnbuffered()
	stopOther, err := events.subscribe(other)
	if err != nil {
		t.Fatal(err)
	}
	defer stopOther()
	for i := 1; i <= 3; i++ {
		events.send(i)
	}
	for _, want := range []int{1, 2} {
		if got := <-buffered; got != want {
			t.Fatalf("buffered = %d, want %d", got, want)
		}
	}
	for _, want := range []int{1, 2, 3} {
		if got := <-other; got != want {
			t.Fatalf("other = %d, want %d", got, want)
		}
	}
	stop()
	replacement, err := events.subscribe(buffered)
	if err != nil {
		t.Fatal(err)
	}
	duplicate()
	stop()
	events.send(4)
	if got := <-buffered; got != 4 {
		t.Fatalf("replacement = %d, want 4", got)
	}
	replacement()
	close(buffered)
	events.send(5)
	events.close()
	other <- 6
	if got := <-other; got != 4 {
		t.Fatalf("release discarded caller buffer: %d", got)
	}
	late := make(chan int, 1)
	if stop, err := events.subscribe(late); err == nil || stop != nil {
		t.Fatal("registration after release succeeded")
	}
	events.send(7)
	select {
	case got := <-late:
		t.Fatalf("delivery after release: %d", got)
	default:
	}
	close(late)
}

func TestNotificationsStopCloseRace(t *testing.T) {
	for range 100 {
		var events notifications[int]
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			for i := range 100 {
				events.send(i)
			}
		}()
		go func() {
			defer workers.Done()
			for i := range 20 {
				if i == 10 {
					events.close()
				}
				c := make(chan int, 1)
				stop, err := events.subscribe(c)
				if err == nil {
					stop()
				} else if i < 10 || stop != nil {
					t.Errorf("registration failed unexpectedly: %v", err)
				}
				close(c)
			}
		}()
		workers.Wait()
		events.close()
	}
}

func TestNotificationsNilChannel(t *testing.T) {
	var events notifications[int]
	if stop, err := events.subscribe(nil); err == nil || stop != nil {
		t.Fatal("nil channel accepted")
	}
}

func TestNotificationRegistrationNilChannel(t *testing.T) {
	machine, err := NewVirtualMachine(newTestConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if stop, err := machine.StateChangedNotify(nil); err == nil || stop != nil {
		t.Fatal("nil state channel accepted")
	}
	if stop, err := machine.ConsolePortChangedNotify(nil); err == nil || stop != nil {
		t.Fatal("nil console channel accepted")
	}
	if stop, err := machine.GraphicsDisplayChangedNotify(nil); err == nil || stop != nil {
		t.Fatal("nil display channel accepted")
	}
}
