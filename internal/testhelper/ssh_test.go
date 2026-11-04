package testhelper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSshSessionCommandDeadline(t *testing.T) {
	client, conn := newSessionTestClient(t, true)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	session, err := NewSshSession(ctx, client, conn)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if err := session.Run("hold"); err == nil {
		t.Fatal("command without an exit status completed successfully")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("command returned before its deadline: %v", ctx.Err())
	}
}

func TestSshSessionOpenDeadline(t *testing.T) {
	client, conn := newSessionTestClient(t, false)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	session, err := NewSshSession(ctx, client, conn)
	if session != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got (%v, %v), want (nil, deadline exceeded)", session, err)
	}
}

func TestSshSessionCloseStopsItsDeadline(t *testing.T) {
	client, conn := newSessionTestClient(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first, err := NewSshSession(ctx, client, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()

	second, err := NewSshSession(t.Context(), client, conn)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Run("true"); err != nil {
		t.Fatalf("closing the previous session affected the next one: %v", err)
	}
}

func TestSshSessionCanceledContext(t *testing.T) {
	client, conn := newSessionTestClient(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	session, err := NewSshSession(ctx, client, conn)
	if session != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("got (%v, %v), want (nil, context canceled)", session, err)
	}

	next, err := NewSshSession(t.Context(), client, conn)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.Run("true"); err != nil {
		t.Fatalf("a canceled session open closed the transport: %v", err)
	}
}

func newSessionTestClient(t *testing.T, acceptSession bool) (*ssh.Client, net.Conn) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		server, channels, _, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		if err := conn.SetDeadline(time.Time{}); err != nil {
			return
		}
		for incoming := range channels {
			if !acceptSession {
				server.Wait()
				return
			}
			channel, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			for request := range requests {
				if request.Type != "exec" {
					request.Reply(false, nil)
					continue
				}
				var command struct {
					Command string
				}
				if err := ssh.Unmarshal(request.Payload, &command); err != nil {
					return
				}
				request.Reply(true, nil)
				if command.Command == "true" {
					status := struct {
						Status uint32
					}{
						Status: 0,
					}
					channel.SendRequest("exit-status", false, ssh.Marshal(status))
					channel.Close()
				}
			}
			channel.Close()
		}
	}()

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close()
		select {
		case <-serverDone:
		case <-time.After(3 * time.Second):
			t.Error("SSH test server did not stop")
		}
	})
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	client, err := NewSshClient(conn, listener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()),
	})
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Time{})
	t.Cleanup(func() { client.Close() })
	return client, conn
}
