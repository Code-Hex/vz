package vz

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestSocketAcceptQueuePreservesConnections(t *testing.T) {
	state := &socketAcceptState{wake: make(chan struct{}, 1)}
	defer state.close()
	for index := range 32 {
		server, client := net.Pipe()
		defer client.Close()
		connection := &VirtioSocketConnection{rawConn: server, sourcePort: uint32(index)}
		if !state.enqueue(connResults{conn: connection}) {
			t.Fatal("open listener rejected a connection")
		}
	}
	for index := range 32 {
		connection, err := state.accept()
		if err != nil || connection.SourcePort() != uint32(index) {
			t.Fatalf("accept %d returned %v, %v", index, connection, err)
		}
		connection.Close()
	}
}

func TestSocketAcceptCloseWakesWaiters(t *testing.T) {
	state := &socketAcceptState{wake: make(chan struct{}, 1)}
	results := make(chan error, 16)
	for range cap(results) {
		go func() {
			_, err := state.accept()
			results <- err
		}()
	}
	state.close()
	state.close()
	for range cap(results) {
		select {
		case err := <-results:
			if !errors.Is(err, net.ErrClosed) {
				t.Fatalf("closed listener returned %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("listener close left an accept blocked")
		}
	}
}

func TestSocketAcceptCloseRacesWithConnections(t *testing.T) {
	state := &socketAcceptState{wake: make(chan struct{}, 1)}
	var wait sync.WaitGroup
	clients := make([]net.Conn, 64)
	for index := range clients {
		server, client := net.Pipe()
		clients[index] = client
		defer client.Close()
		wait.Add(1)
		go func() {
			defer wait.Done()
			state.enqueue(connResults{conn: &VirtioSocketConnection{rawConn: server}})
		}()
	}
	state.close()
	wait.Wait()
	for _, client := range clients {
		client.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
			t.Fatalf("pending connection was not closed: %v", err)
		}
	}
}
