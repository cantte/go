package buffer

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := []Config{
		{
			Name:     "default_settings",
			Capacity: 10_000,
			Drop:     false,
		},
		{
			Name:     "custom_capacity",
			Capacity: 5000,
			Drop:     false,
		},
		{
			Name:     "with_drop_enabled",
			Capacity: 10_000,
			Drop:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			b := New[int](tt)

			assert.Equal(t, tt.Capacity, cap(b.c), "channel capacity should match")
			assert.Equal(t, tt.Drop, b.drop, "drop behavior should match")
		})
	}
}

func TestBuffer(t *testing.T) {
	tests := []struct {
		config  Config
		name    string
		input   []int
		wantLen int
	}{
		{
			config: Config{
				Name:     "a",
				Capacity: 5,
				Drop:     false,
			},
			name:    "Should buffer all elements when under capacity",
			input:   []int{1, 2, 3},
			wantLen: 3,
		},
		{
			config: Config{
				Name:     "b",
				Capacity: 3,
				Drop:     true,
			},
			name:    "Should drop elements when buffer is full and drop is enabled",
			input:   []int{1, 2, 3, 4, 5},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New[int](tt.config)

			// Buffer elements
			for _, v := range tt.input {
				b.Buffer(v)
			}

			assert.Equal(t, tt.wantLen, len(b.c), "buffer length should match expected")

			// Verify elements can be received
			received := make([]int, 0, tt.wantLen)
			timeout := time.After(100 * time.Millisecond)

		receiveLoop:
			for {
				select {
				case v := <-b.c:
					received = append(received, v)
				case <-timeout:
					break receiveLoop
				}
			}

			assert.Equal(t, tt.wantLen, len(received), "received elements count should match expected")
		})
	}
}

func TestBlockingBehavior(t *testing.T) {
	t.Run("blocks_when_full", func(t *testing.T) {
		b := New[int](Config{
			Name:     "a",
			Capacity: 2,
			Drop:     false,
		})
		defer b.Close()

		// Fill the buffer
		b.Buffer(1)
		b.Buffer(2)

		// Try to add another element with timeout
		done := make(chan bool, 1)
		go func() {
			b.Buffer(3)
			done <- true
		}()

		select {
		case <-done:
			t.Error("Buffer should block when full")
		case <-time.After(100 * time.Millisecond):
			// Expected behavior - operation blocked
		}
	})
}

func TestCustomTypes(t *testing.T) {
	type CustomEvent struct {
		ID   string
		Data string
	}

	t.Run("custom_type_buffering", func(t *testing.T) {
		b := New[CustomEvent](Config{
			Name:     "custom_event",
			Capacity: 1000,
			Drop:     false,
		})
		event := CustomEvent{ID: "1", Data: "test"}

		b.Buffer(event)

		select {
		case received := <-b.c:
			assert.Equal(t, event, received, "received event should match buffered event")
		default:
			t.Error("Expected to receive buffered event")
		}
	})
}

func TestBufferCloseConcurrency(t *testing.T) {
	b := New[int](Config{Capacity: 10, Drop: true, Name: "test"})

	bufferingStarted := make(chan struct{})
	bufferingDone := make(chan struct{})

	go func() {
		for i := range 1000 {
			if i == 0 {
				close(bufferingStarted)
			}
			b.Buffer(i)
		}
		close(bufferingDone)
	}()

	go func() {
		<-bufferingStarted
		b.Close()
	}()

	<-bufferingDone

	// Force close the buffer again (should not panic)
	b.Close()
	b.Close()
}

func TestBufferWithMetricsDisabled(t *testing.T) {
	b := New[int](
		Config{Capacity: 1, Drop: true, Name: "metrics_disabled"},
		WithMetrics(false),
	)

	b.Buffer(1)
	b.Buffer(2)
	b.Close()
	b.Buffer(3)

	if got := b.Size(); got != 1 {
		t.Fatalf("Size() = %d, want 1", got)
	}
}

func TestCloseReleasesBlockedProducer(t *testing.T) {
	b := New[int](
		Config{Capacity: 1, Drop: false, Name: "blocked_producer"},
		WithMetrics(false),
	)
	b.Buffer(1)

	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		b.Buffer(2)
	}()

	// Give the producer an opportunity to block on the full channel.
	time.Sleep(10 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		b.Close()
	}()

	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("blocked producer was not released by Close")
	}

	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close blocked while a producer was waiting")
	}

	values := 0
	for range b.Consume() {
		values++
	}
	if values != 1 {
		t.Fatalf("consumed %d values, want the one buffered before Close", values)
	}
}

func TestNewRejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{-1, 0} {
		t.Run(strconv.Itoa(capacity), func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatalf("New with capacity %d did not panic", capacity)
				}
				if message := fmt.Sprint(recovered); message != "buffer: capacity must be greater than zero" {
					t.Fatalf("panic = %q, want clear capacity error", message)
				}
			}()

			New[int](Config{Capacity: capacity}, WithMetrics(false))
		})
	}
}

func TestDropFullDoesNotAllocateItem(t *testing.T) {
	b := New[int](
		Config{Capacity: 1, Drop: true, Name: "full_no_allocation"},
		WithMetrics(false),
	)
	defer b.Close()
	b.Buffer(1)

	allocations := testing.AllocsPerRun(100, func() {
		b.Buffer(2)
	})
	if allocations != 0 {
		t.Fatalf("full-buffer drop allocated %v objects per call, want 0", allocations)
	}
}

func TestBufferValueDoesNotAllocate(t *testing.T) {
	b := New[int](
		Config{Capacity: 1, Drop: true, Name: "value_no_allocation"},
		WithMetrics(false),
	)
	defer b.Close()

	allocations := testing.AllocsPerRun(100, func() {
		b.Buffer(1)
		<-b.Consume()
	})
	if allocations != 0 {
		t.Fatalf("buffering a value allocated %v objects per call, want 0", allocations)
	}
}

// BenchmarkBuffer measures the performance impact of mutex operations
// in different scenarios to understand the cost of thread safety
func BenchmarkBuffer(b *testing.B) {
	benchmarks := []struct {
		name     string
		capacity int
		drop     bool
		workers  int
	}{
		{"SingleThread_NoDrop_Small", 100, false, 1},
		{"SingleThread_NoDrop_Large", 10000, false, 1},
		{"SingleThread_Drop_Small", 100, true, 1},
		{"SingleThread_Drop_Large", 10000, true, 1},
		{"MultiThread_NoDrop_2Workers", 1000, false, 2},
		{"MultiThread_NoDrop_4Workers", 1000, false, 4},
		{"MultiThread_NoDrop_8Workers", 1000, false, 8},
		{"MultiThread_Drop_2Workers", 1000, true, 2},
		{"MultiThread_Drop_4Workers", 1000, true, 4},
		{"MultiThread_Drop_8Workers", 1000, true, 8},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			buf := New[int](Config{
				Capacity: bm.capacity,
				Drop:     bm.drop,
				Name:     "benchmark",
			})
			defer buf.Close()

			// Start consumer to prevent blocking
			go func() {
				for range buf.Consume() {
					// Consume all items
				}
			}()

			b.ResetTimer()
			b.ReportAllocs()

			if bm.workers == 1 {
				// Single-threaded benchmark
				for i := 0; i < b.N; i++ {
					buf.Buffer(i)
				}
			} else {
				// Multi-threaded benchmark
				b.RunParallel(func(pb *testing.PB) {
					i := 0
					for pb.Next() {
						buf.Buffer(i)
						i++
					}
				})
			}
		})
	}
}

// BenchmarkBufferMutexContention specifically measures mutex contention
// by comparing scenarios with high and low contention
func BenchmarkBufferMutexContention(b *testing.B) {
	scenarios := []struct {
		name        string
		bufferCount int
		workerCount int
	}{
		{"LowContention_1Buffer_2Workers", 1, 2},
		{"HighContention_1Buffer_8Workers", 1, 8},
		{"LowContention_4Buffers_8Workers", 4, 8},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			buffers := make([]*Buffer[int], scenario.bufferCount)
			for i := range buffers {
				buffers[i] = New[int](Config{
					Capacity: 1000,
					Drop:     true,
					Name:     "contention_test",
				})
				defer buffers[i].Close()

				// Start consumer for each buffer
				go func(buf *Buffer[int]) {
					for range buf.Consume() {
						// Consume all items
					}
				}(buffers[i])
			}

			b.ResetTimer()
			b.ReportAllocs()

			b.RunParallel(func(pb *testing.PB) {
				bufferIndex := 0
				itemCount := 0
				for pb.Next() {
					// Distribute work across buffers
					buffers[bufferIndex%len(buffers)].Buffer(itemCount)
					bufferIndex++
					itemCount++
				}
			})
		})
	}
}

// BenchmarkBufferClose measures the performance of closing operations
// under different concurrency scenarios
func BenchmarkBufferClose(b *testing.B) {
	b.Run("CloseWithoutConcurrency", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := New[int](Config{Capacity: 100, Drop: true, Name: "close_test"})
			buf.Close()
		}
	})

	b.Run("CloseWithConcurrentWrites", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := New[int](Config{Capacity: 100, Drop: true, Name: "close_test"})

			done := make(chan struct{})
			go func() {
				defer close(done)
				for j := 0; j < 100; j++ {
					buf.Buffer(j)
				}
			}()

			// Close while writes are happening
			buf.Close()
			<-done
		}
	})
}

// BenchmarkBufferMemoryFootprint measures memory usage
func BenchmarkBufferMemoryFootprint(b *testing.B) {
	b.Run("MemoryUsage", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			buf := New[int](Config{Capacity: 1000, Drop: false, Name: "memory_test"})
			for j := 0; j < 100; j++ {
				buf.Buffer(j)
			}
			buf.Close()
		}
	})
}
