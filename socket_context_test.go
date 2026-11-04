//go:build darwin

package vz

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestSocketConnectContextCanceledBeforeSubmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var device *VirtioSocketDevice
	connection, err := device.ConnectContext(ctx, 1234)
	if connection != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("got (%v, %v), want (nil, context.Canceled)", connection, err)
	}
}

func TestSocketConnectRequestCancellationClosesConnection(t *testing.T) {
	for _, test := range []struct {
		name   string
		queued bool
	}{
		{
			name: "late completion",
		},
		{
			name:   "queued completion",
			queued: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			local, peer := net.Pipe()
			defer local.Close()
			defer peer.Close()
			if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			request := socketConnectRequest{
				result: make(chan connResults, 1),
			}
			result := connResults{
				conn: &VirtioSocketConnection{
					rawConn: local,
				},
			}
			if test.queued {
				request.complete(result)
				request.cancel()
			} else {
				request.cancel()
				request.complete(result)
			}
			if _, err := peer.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
				t.Fatalf("peer read = %v, want EOF", err)
			}
		})
	}
}

func TestSocketConnectRequestCancellationRace(t *testing.T) {
	for range 50 {
		local, peer := net.Pipe()
		if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			local.Close()
			peer.Close()
			t.Fatal(err)
		}
		request := socketConnectRequest{
			result: make(chan connResults, 1),
		}
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			request.complete(connResults{
				conn: &VirtioSocketConnection{
					rawConn: local,
				},
			})
		}()
		go func() {
			defer wait.Done()
			request.cancel()
		}()
		wait.Wait()
		_, err := peer.Read(make([]byte, 1))
		local.Close()
		peer.Close()
		if !errors.Is(err, io.EOF) {
			t.Fatalf("peer read = %v, want EOF", err)
		}
	}
}

func TestSocketConnectRequestTransfersConnection(t *testing.T) {
	local, peer := net.Pipe()
	defer local.Close()
	defer peer.Close()
	request := socketConnectRequest{
		result: make(chan connResults, 1),
	}
	request.complete(connResults{
		conn: &VirtioSocketConnection{
			rawConn: local,
		},
	})
	result := <-request.result
	request.cancel()
	if err := result.conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	written := make(chan error, 1)
	go func() {
		_, err := peer.Write([]byte("x"))
		written <- err
	}()
	buffer := make([]byte, 1)
	if _, err := result.conn.Read(buffer); err != nil || string(buffer) != "x" {
		t.Fatalf("read = %q, %v, want x", buffer, err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
}

func TestSocketConnectRequestError(t *testing.T) {
	want := errors.New("connection failed")
	request := socketConnectRequest{
		result: make(chan connResults, 1),
	}
	request.complete(connResults{
		err: want,
	})
	result := <-request.result
	if result.conn != nil || !errors.Is(result.err, want) {
		t.Fatalf("got (%v, %v), want (nil, %v)", result.conn, result.err, want)
	}
	request.cancel()
	request.complete(connResults{
		err: want,
	})
}
