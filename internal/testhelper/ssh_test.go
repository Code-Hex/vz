package testhelper

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSessionKeepAliveAfterClientClose(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			serverDone <- err
			return
		}
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			serverDone <- err
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			channel, requests, err := incoming.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			defer channel.Close()
			go ssh.DiscardRequests(requests)
		}
		serverDone <- nil
	}()

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	client, err := NewSshClient(conn, listener.Addr().String(), &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	_ = client.Wait()
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := session.SendRequest("keepalive@codehex.vz", true, nil)
		result <- err
	}()
	select {
	case err := <-result:
		if err != io.EOF {
			t.Fatalf("keep-alive after client close returned %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("keep-alive request did not return after client close")
	}
}
