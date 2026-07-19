package buffer

import (
	"sync"
	"time"

	"github.com/cantte/go/buffer/metrics"
	"github.com/cantte/go/repeat"
)

// Buffer represents a generic buffered channel that can store elements of type T.
// It provides configuration for capacity and drop behavior when the buffer is full.
type Buffer[T any] struct {
	c    chan T
	drop bool   // Whether to drop new elements when buffer is full
	name string // name of the buffer

	stopMetrics func()
	closeOnce   sync.Once
	mu          sync.Mutex
	isClosed    bool
	stop        chan struct{}
	senders     sync.WaitGroup

	metrics *metrics.BufferMetrics
}

type Config struct {
	Capacity int    // Maximum number of elements the buffer can hold
	Drop     bool   // Whether to drop new elements when buffer is full
	Name     string // name of the buffer
}

type BufferOption func(*bufferOption)

type bufferOption struct {
	metricsEnabled   bool
	metricsNamespace string
}

// WithMetricsNamespace sets the Prometheus namespace used by buffer
// metrics. An empty namespace means that metric names have no namespace
// prefix.
func WithMetricsNamespace(namespace string) BufferOption {
	return func(config *bufferOption) {
		config.metricsNamespace = namespace
	}
}

// WithMetrics enables or disables Prometheus buffer metrics.
func WithMetrics(enabled bool) BufferOption {
	return func(config *bufferOption) {
		config.metricsEnabled = enabled
	}
}

// New creates a new Buffer with the specified configuration.
// The Config.Capacity field determines the maximum number of elements the buffer can hold.
// The Config.Drop field determines whether new elements should be dropped when the buffer is full.
// The Config.Name field provides an identifier for metrics and logging.
//
// Example:
//
//	// Create a buffer for integers with capacity 1000 and no drop behavior
//	intBuffer := buffer.New[int](buffer.Config{
//		Capacity: 1000,
//		Drop:     false,
//		Name:     "int_buffer",
//	})
//
//	// Create a buffer for strings with capacity 500 that drops when full
//	stringBuffer := buffer.New[string](buffer.Config{
//		Capacity: 500,
//		Drop:     true,
//		Name:     "string_buffer",
//	})
func New[T any](config Config, options ...BufferOption) *Buffer[T] {
	if config.Capacity <= 0 {
		panic("buffer: capacity must be greater than zero")
	}

	opt := bufferOption{metricsEnabled: true}
	for _, option := range options {
		if option != nil {
			option(&opt)
		}
	}

	b := &Buffer[T]{
		c:           make(chan T, config.Capacity),
		drop:        config.Drop,
		name:        config.Name,
		stop:        make(chan struct{}),
		stopMetrics: func() {},
	}

	if opt.metricsEnabled {
		b.metrics = metrics.New(opt.metricsNamespace).ForBuffer(b.name, b.drop)

		b.stopMetrics = repeat.Every(time.Minute, func() {
			b.recordFillRatio()
		})
	}

	return b
}

// Buffer adds an element to the buffer.
// If drop is enabled and the buffer is full, the element will be discarded.
// If drop is disabled and the buffer is full, this operation will block until space is available.
//
// Example:
//
//	// Create a buffer with capacity 1000 and no drop behavior
//	buffer := buffer.New[int](buffer.Config{
//		Capacity: 1000,
//		Drop:     false,
//		Name:     "int_buffer",
//	})
//
//	// Add integer to buffer
//	buffer.Buffer(42)
//
//	// Example with custom type
//	type Event struct {
//	    ID   string
//	    Data string
//	}
//	eventBuffer := buffer.New[Event](buffer.Config{
//		Capacity: 1000,
//		Drop:     false,
//		Name:     "event_buffer",
//	})
//	eventBuffer.Buffer(Event{ID: "1", Data: "example"})
func (b *Buffer[T]) Buffer(t T) {
	b.mu.Lock()
	if b.isClosed {
		b.mu.Unlock()
		b.metrics.Closed()
		return
	}
	b.senders.Add(1)
	b.mu.Unlock()
	defer b.senders.Done()

	if b.drop {
		// Avoid allocating an item that is already known to be unbufferable.
		if len(b.c) == cap(b.c) {
			b.metrics.Dropped()
			return
		}

		select {
		case b.c <- t:
			b.metrics.Buffered()
			b.recordFillRatio()
		case <-b.stop:
			b.metrics.Closed()
		default:
			b.metrics.Dropped()
		}
	} else {
		select {
		case b.c <- t:
			b.metrics.Buffered()
			b.recordFillRatio()
		case <-b.stop:
			b.metrics.Closed()
		}
	}
}

func (b *Buffer[T]) recordFillRatio() {
	b.metrics.SetFillRatio(float64(len(b.c)) / float64(cap(b.c)))
}

// Consume returns a receive-only channel that can be used to read elements from the buffer.
// Elements are removed from the buffer as they are read from the channel.
// The channel will remain open until the Buffer.Close() method is called.
//
// Example:
//
//	buffer := buffer.New[int](buffer.Config{
//		Capacity: 1000,
//		Drop:     false,
//		Name:     "int_buffer",
//	})
//
//	// Consume elements in a separate goroutine
//	go func() {
//	    for event := range buffer.Consume() {
//	        // Process each event
//	        fmt.Println(*event)
//	    }
//	}()
func (b *Buffer[T]) Consume() <-chan T {
	return b.c
}

// Size returns a non-blocking, thread-safe snapshot of the number of buffered elements.
// This value may change immediately due to concurrent sends/receives, so it should
// only be used for monitoring or debugging purposes, not for control flow decisions.
//
// Example:
//
//	size := b.Size()
//	fmt.Printf("Buffer snapshot shows %d elements\n", size)
func (b *Buffer[T]) Size() int {
	return len(b.c)
}

// Close closes the buffer and signals that no more elements will be added.
// This method should be called when the buffer is no longer needed.
//
// Example:
//
//	b := buffer.New[int](buffer.Config{
//		Capacity: 1000,
//		Drop:     false,
//		Name:     "int_buffer",
//	})
//
//	// Close the buffer when done
//	b.Close()
func (b *Buffer[T]) Close() {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.isClosed = true
		close(b.stop)
		b.mu.Unlock()

		// Closing stop releases blocking producers. Waiting for all admitted
		// producers prevents a send/close race on the data channel.
		b.senders.Wait()
		close(b.c)
		b.stopMetrics()
		b.metrics.SetFillRatio(0)
	})
}
