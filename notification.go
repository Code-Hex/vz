//go:build darwin

package vz

// notificationQueue retains notifications until they are read or the owner is released.
// The owner must finish sending before it closes the queue.
type notificationQueue[T any] struct {
	input  chan T
	output chan T
	stop   chan struct{}
	done   chan struct{}
}

func newNotificationQueue[T any]() *notificationQueue[T] {
	q := &notificationQueue[T]{
		input:  make(chan T),
		output: make(chan T),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go q.run()
	return q
}

func (q *notificationQueue[T]) send(value T) {
	q.input <- value
}

func (q *notificationQueue[T]) close() {
	close(q.stop)
	<-q.done
}

func (q *notificationQueue[T]) run() {
	defer close(q.done)
	defer close(q.output)
	var pending []T
	for {
		var output chan T
		var next T
		if len(pending) != 0 {
			output = q.output
			next = pending[0]
		}
		select {
		case <-q.stop:
			return
		case value := <-q.input:
			pending = append(pending, value)
		case output <- next:
			var zero T
			pending[0] = zero
			pending = pending[1:]
			if len(pending) == 0 {
				pending = nil
			}
		}
	}
}
