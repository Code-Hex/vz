//go:build darwin

package vz

import (
	"errors"
	"sync"
)

type notificationRegistration[T any] struct {
	channel chan<- T
}

type notifications[T any] struct {
	mu       sync.Mutex
	channels map[chan<- T]*notificationRegistration[T]
	closed   bool
}

func (n *notifications[T]) subscribe(channel chan<- T) (func(), error) {
	if channel == nil {
		return nil, errors.New("vz: nil notification channel")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil, errors.New("vz: notification source is closed")
	}
	if n.channels == nil {
		n.channels = make(map[chan<- T]*notificationRegistration[T])
	}
	registration := n.channels[channel]
	if registration == nil {
		registration = &notificationRegistration[T]{channel: channel}
		n.channels[channel] = registration
	}
	return func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.channels[registration.channel] == registration {
			delete(n.channels, registration.channel)
		}
	}, nil
}

func (n *notifications[T]) send(value T) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for channel := range n.channels {
		select {
		case channel <- value:
		default:
		}
	}
}

func (n *notifications[T]) close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closed = true
	n.channels = nil
}
