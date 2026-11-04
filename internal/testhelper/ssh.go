package testhelper

import (
	"context"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

func NewSshConfig(username, password string) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

func NewSshClient(conn net.Conn, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// Session closes its SSH transport if the session context ends.
type Session struct {
	*ssh.Session
	stop      func()
	closeOnce sync.Once
	closeErr  error
}

// NewSshSession opens a session and closes conn if ctx ends before Close.
func NewSshSession(ctx context.Context, client *ssh.Client, conn net.Conn) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	stopCallback := context.AfterFunc(ctx, func() {
		conn.Close()
		close(done)
	})
	stop := func() {
		if stopCallback() {
			close(done)
		}
		<-done
	}
	session, err := client.NewSession()
	if err != nil {
		contextErr := ctx.Err()
		stop()
		if contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	return &Session{
		Session: session,
		stop:    stop,
	}, nil
}

// Close closes the session and joins any active cancellation callback.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.Session.Close()
		s.stop()
	})
	return s.closeErr
}
