package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cantte/go/clock"
	"github.com/stretchr/testify/require"
)

var errTestDownstream = errors.New("downstream test error")

func TestCircuitBreakerStates(t *testing.T) {

	c := clock.NewTestClock()
	cb := New[int]("test", WithCyclicPeriod(5*time.Second), WithClock(c), WithTripThreshold(3))

	// Test Closed State
	for i := 0; i < 3; i++ {
		_, err := cb.Do(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errTestDownstream
		})
		require.ErrorIs(t, err, errTestDownstream)
	}
	require.Equal(t, Open, cb.state)

	// Test Open State
	_, err := cb.Do(context.Background(), func(ctx context.Context) (int, error) {
		return 0, errTestDownstream
	})
	require.ErrorIs(t, err, ErrTripped)
	require.Equal(t, Open, cb.state)

	// Test Half-Open State
	c.Tick(2 * time.Minute) // Advance time to reset
	_, err = cb.Do(context.Background(), func(ctx context.Context) (int, error) {
		return 42, nil
	})
	require.NoError(t, err)
	require.Equal(t, HalfOpen, cb.state)
}

func TestCircuitBreakerFailureRatio(t *testing.T) {
	run := func(cb *CB[int], fail bool) {
		_, _ = cb.Do(context.Background(), func(ctx context.Context) (int, error) {
			if fail {
				return 0, errTestDownstream
			}
			return 0, nil
		})
	}

	t.Run("low failure rate does not trip even with many absolute failures", func(t *testing.T) {
		c := clock.NewTestClock()
		// 50% ratio, but a high-throughput window: 1000 ok + 5 fail = 0.5% << 50%.
		cb := New[int]("test", WithClock(c), WithCyclicPeriod(time.Hour),
			WithFailureRatio(0.5, 20))
		for i := 0; i < 1000; i++ {
			run(cb, false)
		}
		for i := 0; i < 5; i++ {
			run(cb, true)
		}
		require.Equal(t, Closed, cb.state, "0.5%% failure rate must not trip a 50%% breaker")
	})

	t.Run("high failure rate trips once past minRequests", func(t *testing.T) {
		c := clock.NewTestClock()
		cb := New[int]("test", WithClock(c), WithCyclicPeriod(time.Hour),
			WithFailureRatio(0.5, 20))
		// Below minRequests: all failing but sample too small to act.
		for i := 0; i < 19; i++ {
			run(cb, true)
		}
		require.Equal(t, Closed, cb.state, "must not trip before minRequests")
		run(cb, true) // 20th failing request crosses minRequests at 100% failure
		require.Equal(t, Open, cb.state, "100%% failure past minRequests must trip")
	})

	t.Run("ratio above 1 is clamped to 1 and still trips at 100% failure", func(t *testing.T) {
		c := clock.NewTestClock()
		// A typo'd ratio of 1.5 must not silently produce a never-tripping breaker.
		cb := New[int]("test", WithClock(c), WithCyclicPeriod(time.Hour),
			WithFailureRatio(1.5, 5))
		require.Equal(t, 1.0, cb.config.failureRatio, "ratio above 1 must clamp to 1")
		for i := 0; i < 5; i++ {
			run(cb, true)
		}
		require.Equal(t, Open, cb.state, "100%% failure past minRequests must trip a clamped breaker")
	})
}

func TestCircuitBreakerReset(t *testing.T) {

	c := clock.NewTestClock()
	cb := New[int]("test", WithCyclicPeriod(5*time.Second), WithClock(c), WithTripThreshold(3), WithTimeout(20*time.Second))

	// Trigger circuit breaker to open
	for i := 0; i < 3; i++ {
		_, err := cb.Do(context.Background(), func(ctx context.Context) (int, error) {
			return 0, errTestDownstream
		})
		require.ErrorIs(t, err, errTestDownstream)
	}

	require.Equal(t, Open, cb.state)

	// Advance time to reset
	c.Tick(30 * time.Second)

	// Next request should be allowed (Half-Open state)
	_, err := cb.Do(context.Background(), func(ctx context.Context) (int, error) {
		return 42, nil
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	require.Equal(t, HalfOpen, cb.state)

}

func TestCircuitBreakerRecovers(t *testing.T) {

	cb := New[int]("test", WithMaxRequests(2))

	// Reset to Half-Open state
	cb.state = HalfOpen

	// Two requests should succeed
	for i := 0; i < 2; i++ {
		_, err := cb.Do(context.Background(), func(ctx context.Context) (int, error) {
			return 42, nil
		})
		require.NoError(t, err)
	}

	// Circuit should close
	require.Equal(t, Closed, cb.state)
}

func TestRecoveryTimeoutStartsWhenCircuitOpens(t *testing.T) {
	c := clock.NewTestClock()
	cb := New[int]("test", WithClock(c), WithTripThreshold(1), WithTimeout(time.Minute))

	// Let more than a timeout pass before the circuit trips.
	c.Tick(2 * time.Minute)
	_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
		return 0, errTestDownstream
	})
	require.ErrorIs(t, err, errTestDownstream)
	require.Equal(t, Open, cb.state)

	_, err = cb.Do(context.Background(), func(context.Context) (int, error) {
		return 0, nil
	})
	require.ErrorIs(t, err, ErrTripped)

	c.Tick(time.Minute + time.Nanosecond)
	_, err = cb.Do(context.Background(), func(context.Context) (int, error) {
		return 0, nil
	})
	require.NoError(t, err)
	require.Equal(t, HalfOpen, cb.state)
}

func TestHalfOpenReservesProbeSlotsBeforeExecution(t *testing.T) {
	cb := New[int]("test", WithMaxRequests(1))
	cb.state = HalfOpen

	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error)
	go func() {
		_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
			close(started)
			<-release
			return 0, nil
		})
		firstDone <- err
	}()
	<-started

	var calls atomic.Int32
	_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
		calls.Add(1)
		return 0, nil
	})
	require.ErrorIs(t, err, ErrTooManyRequests)
	require.Zero(t, calls.Load())

	close(release)
	require.NoError(t, <-firstDone)
}

func TestHalfOpenConcurrentProbeLimit(t *testing.T) {
	const maxRequests = 3
	cb := New[int]("test", WithMaxRequests(maxRequests))
	cb.state = HalfOpen

	release := make(chan struct{})
	var calls atomic.Int32
	// admitted counts goroutines that have cleared preflight, whether they were
	// let through or rejected. Waiting on calls alone is not enough: releasing
	// the probes lets them succeed and close the circuit, after which any
	// goroutine that had not yet reached preflight sails through as a normal
	// Closed-state request and inflates calls.
	var admitted atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
				calls.Add(1)
				admitted.Add(1)
				<-release
				return 0, nil
			})
			if err != nil {
				admitted.Add(1)
			}
			errs <- err
		}()
	}

	require.Eventually(t, func() bool {
		return admitted.Load() == 20
	}, time.Second, time.Millisecond)
	require.EqualValues(t, maxRequests, calls.Load())
	close(release)
	wg.Wait()
	close(errs)

	require.EqualValues(t, maxRequests, calls.Load())
	tooMany := 0
	for err := range errs {
		if errors.Is(err, ErrTooManyRequests) {
			tooMany++
		}
	}
	require.Equal(t, 20-maxRequests, tooMany)
}

func TestHalfOpenFailureReopensCircuit(t *testing.T) {
	c := clock.NewTestClock()
	cb := New[int]("test", WithClock(c), WithMaxRequests(2), WithTimeout(time.Minute))
	cb.state = HalfOpen

	_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
		return 0, errTestDownstream
	})
	require.ErrorIs(t, err, errTestDownstream)
	require.Equal(t, Open, cb.state)

	_, err = cb.Do(context.Background(), func(context.Context) (int, error) {
		return 0, nil
	})
	require.ErrorIs(t, err, ErrTripped)
}

func TestPanicCountsAsFailureAndReleasesProbeSlot(t *testing.T) {
	callPanics := func(cb *CB[int]) {
		defer func() {
			require.NotNil(t, recover(), "panic must propagate to the caller")
		}()
		_, _ = cb.Do(context.Background(), func(context.Context) (int, error) {
			panic("boom")
		})
	}

	t.Run("panicking dependency trips a closed circuit", func(t *testing.T) {
		c := clock.NewTestClock()
		cb := New[int]("test", WithClock(c), WithCyclicPeriod(time.Hour), WithTripThreshold(2))

		callPanics(cb)
		require.Equal(t, Closed, cb.state)
		callPanics(cb)
		require.Equal(t, Open, cb.state, "panics must count toward the trip threshold")
	})

	t.Run("panicking probe does not leak its half-open slot", func(t *testing.T) {
		c := clock.NewTestClock()
		cb := New[int]("test", WithClock(c), WithMaxRequests(1), WithTimeout(time.Minute))
		cb.state = HalfOpen

		callPanics(cb)
		// A panicking probe is a failed probe: the circuit reopens rather than
		// staying half-open with its only slot permanently consumed.
		require.Equal(t, Open, cb.state)
		require.Zero(t, cb.halfOpenRequests)

		c.Tick(time.Minute + time.Nanosecond)
		_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
			return 42, nil
		})
		require.NoError(t, err, "the breaker must still be able to probe for recovery")
	})
}

func TestStaleResultDoesNotAffectLaterState(t *testing.T) {
	c := clock.NewTestClock()
	cb := New[int]("test", WithClock(c), WithTripThreshold(1), WithTimeout(time.Minute), WithMaxRequests(1))

	// Admit a request while the circuit is still closed, and hold it in flight.
	// admitted is closed from inside fn, so waiting on it proves the request
	// cleared preflight before anything below changes the breaker's state.
	release := make(chan struct{})
	admitted := make(chan struct{})
	straggler := make(chan struct{})
	go func() {
		defer close(straggler)
		_, _ = cb.Do(context.Background(), func(context.Context) (int, error) {
			close(admitted)
			<-release
			return 0, errTestDownstream
		})
	}()
	<-admitted

	// Trip the circuit with an unrelated failure and wait out the timeout.
	_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
		return 0, errTestDownstream
	})
	require.ErrorIs(t, err, errTestDownstream)
	require.Equal(t, Open, cb.state)
	c.Tick(2 * time.Minute)

	// A genuine probe is admitted; the downstream has recovered.
	_, err = cb.Do(context.Background(), func(context.Context) (int, error) {
		return 42, nil
	})
	require.NoError(t, err)
	require.Equal(t, Closed, cb.state, "a single successful probe closes a maxRequests=1 breaker")

	// The straggler from the closed-era window now fails. It must not be
	// applied: it describes a state the breaker has already left.
	close(release)
	<-straggler
	require.Equal(t, Closed, cb.state, "a stale in-flight failure must not reopen the circuit")
}

func TestSlowRequestsSpanningCyclicPeriodStillTrip(t *testing.T) {
	c := clock.NewTestClock()
	// Requests that outlive the cyclic period are exactly what a hung
	// dependency looks like. A cyclic reset is not a state change, so their
	// results must still count when they land, otherwise a dependency slower
	// than cyclicPeriod could never trip the breaker.
	cb := New[int]("test", WithClock(c), WithCyclicPeriod(time.Second), WithTripThreshold(2))

	release := make(chan struct{})
	inFlight := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = cb.Do(context.Background(), func(context.Context) (int, error) {
				inFlight <- struct{}{}
				<-release
				return 0, errTestDownstream
			})
		}()
	}
	for i := 0; i < 2; i++ {
		<-inFlight
	}

	// Cross the cyclic boundary and let an unrelated request drive the reset.
	c.Tick(2 * time.Second)
	_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
		return 42, nil
	})
	require.NoError(t, err)

	close(release)
	wg.Wait()
	require.Equal(t, Open, cb.state, "results of requests that outlived a cyclic reset must still count")
}

func TestOptionsClampUnusableValues(t *testing.T) {
	t.Run("maxRequests below one cannot wedge half-open", func(t *testing.T) {
		cb := New[int]("test", WithMaxRequests(0))
		cb.state = HalfOpen

		_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
			return 42, nil
		})
		require.NoError(t, err, "a zero probe budget would reject every request forever")
		require.Equal(t, Closed, cb.state)
	})

	t.Run("tripThreshold below one does not open on success", func(t *testing.T) {
		cb := New[int]("test", WithTripThreshold(0))

		_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
			return 42, nil
		})
		require.NoError(t, err)
		require.Equal(t, Closed, cb.state, "a successful request must never open the circuit")
	})

	t.Run("failureRatio minRequests below one is clamped", func(t *testing.T) {
		cb := New[int]("test", WithFailureRatio(0.5, 0))
		require.Equal(t, 1, cb.config.minRequests)
	})

	t.Run("non-positive timeout keeps the circuit open", func(t *testing.T) {
		c := clock.NewTestClock()
		cb := New[int]("test", WithClock(c), WithTripThreshold(1), WithTimeout(0))

		_, err := cb.Do(context.Background(), func(context.Context) (int, error) {
			return 0, errTestDownstream
		})
		require.ErrorIs(t, err, errTestDownstream)
		require.Equal(t, Open, cb.state)

		_, err = cb.Do(context.Background(), func(context.Context) (int, error) {
			return 42, nil
		})
		require.ErrorIs(t, err, ErrTripped, "a zero timeout must not defeat fail-fast")
	})

	t.Run("non-positive cyclic period still allows tripping", func(t *testing.T) {
		c := clock.NewTestClock()
		cb := New[int]("test", WithClock(c), WithCyclicPeriod(0), WithTripThreshold(2))

		for i := 0; i < 2; i++ {
			_, _ = cb.Do(context.Background(), func(context.Context) (int, error) {
				return 0, errTestDownstream
			})
		}
		require.Equal(t, Open, cb.state)
	})

	t.Run("nil classifier and clock fall back to defaults", func(t *testing.T) {
		cb := New[int]("test", WithIsDownstreamError(nil), WithClock(nil), WithTripThreshold(1))

		require.NotPanics(t, func() {
			_, _ = cb.Do(context.Background(), func(context.Context) (int, error) {
				return 0, errTestDownstream
			})
		})
		require.Equal(t, Open, cb.state)
	})
}
