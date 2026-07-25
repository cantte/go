package circuitbreaker

import (
	"context"
	"sync"
	"time"

	"github.com/cantte/go/circuitbreaker/metrics"
	"github.com/cantte/go/clock"
	"github.com/cantte/go/logger"
)

// CB is the concrete implementation of [CircuitBreaker]. It tracks request
// success and failure counts to determine when to trip the circuit. CB is
// safe for concurrent use; all state mutations are protected by an embedded
// mutex. Use [New] to create a properly initialized instance.
type CB[Res any] struct {
	sync.Mutex
	// This is a pointer to the configuration of the circuit breaker because we
	// need to modify the clock for testing
	config *config

	// State of the circuit
	state State

	// generation increments on every state transition. Each admitted request
	// carries the generation it was admitted under, and its result is discarded
	// if the breaker has since transitioned. Without this, a straggler admitted
	// while Closed could complete after the breaker reached HalfOpen and
	// re-open the circuit without ever having been a probe, delaying recovery
	// by a full timeout for every in-flight request. Cyclic counter resets do
	// not bump the generation: a slow request that spans a cycle boundary is
	// still a valid observation about the downstream and must keep counting,
	// otherwise a dependency slower than cyclicPeriod would never trip.
	generation uint64

	// reset the counters every cyclic period
	resetCountersAt time.Time

	// reset the state every recoveryTimeout
	resetStateAt time.Time

	// counters are protected by the mutex and are reset every cyclic period
	requests             int
	successes            int
	failures             int
	consecutiveSuccesses int
	consecutiveFailures  int
	// halfOpenRequests reserves probe capacity before downstream calls start.
	halfOpenRequests int
}

type config struct {
	name string
	// Max requests that may pass through the circuit breaker in its half-open state
	// If all requests are successful, the circuit will close
	// If any request fails, the circuit will remaing half open until the next cycle
	maxRequests int

	// Interval to clear counts while the circuit is closed
	cyclicPeriod time.Duration

	// How long the circuit will stay open before transitioning to half-open
	timeout time.Duration

	// Determine whether the error is a downstream error or not
	// If the error is a downstream error, the circuit will count it
	// If the error is not a downstream error, the circuit will not count it
	isDownstreamError func(error) bool

	// How many downstream errors within a cyclic period are allowed before the
	// circuit trips and opens
	tripThreshold int

	// failureRatio, when > 0, switches tripping from an absolute count to a
	// rate: the circuit opens once failures/requests within the cyclic period
	// reaches this fraction, but only after at least minRequests requests so a
	// tiny sample can't trip it. This is robust to traffic volume — a brief
	// sub-1% blip never trips, while a real outage (most requests failing)
	// trips fast regardless of throughput. When 0, tripThreshold is used.
	failureRatio float64

	// minRequests is the minimum number of requests within the cyclic period
	// before failureRatio is evaluated. Ignored when failureRatio is 0.
	minRequests int

	// Clock to use for timing, defaults to the system clock but can be overridden for testing
	clock clock.Clock
}

// WithMaxRequests sets the maximum number of requests allowed through during
// the [HalfOpen] state. If all probe requests succeed, the circuit closes.
// Defaults to 10. Values below 1 are clamped to 1: a breaker that admits zero
// probes could never leave [HalfOpen] and would reject every request forever.
func WithMaxRequests(maxRequests int) applyConfig {
	return func(c *config) {
		if maxRequests < 1 {
			maxRequests = 1
		}
		c.maxRequests = maxRequests
	}
}

// WithCyclicPeriod sets the interval at which failure counts are reset while
// the circuit is [Closed]. A shorter period makes the circuit less sensitive
// to sporadic failures. Defaults to 5 seconds. Non-positive values are ignored
// and leave the default in place, because a zero period would clear the counts
// before every request and the circuit could never reach its trip threshold.
func WithCyclicPeriod(cyclicPeriod time.Duration) applyConfig {
	return func(c *config) {
		if cyclicPeriod <= 0 {
			return
		}
		c.cyclicPeriod = cyclicPeriod
	}
}

// WithIsDownstreamError provides a function to classify errors. Only errors
// where this function returns true count toward the trip threshold. By default,
// all non-nil errors are considered downstream errors. A nil function is
// ignored so the default classifier remains in place rather than panicking on
// the first request.
func WithIsDownstreamError(isDownstreamError func(error) bool) applyConfig {
	return func(c *config) {
		if isDownstreamError == nil {
			return
		}
		c.isDownstreamError = isDownstreamError
	}
}

// WithTripThreshold sets the number of failures within a cyclic period that
// will cause the circuit to trip from [Closed] to [Open]. Defaults to 5.
// Values below 1 are clamped to 1: a threshold of zero is satisfied by zero
// failures and would open the circuit after the first successful request.
func WithTripThreshold(tripThreshold int) applyConfig {
	return func(c *config) {
		if tripThreshold < 1 {
			tripThreshold = 1
		}
		c.tripThreshold = tripThreshold
	}
}

// WithFailureRatio switches tripping from an absolute failure count to a
// failure rate. The circuit opens once failures/requests within the cyclic
// period reaches ratio (0..1], but only after at least minRequests requests so
// a small sample can't trip it. Prefer this for high-throughput call sites
// where a fixed count is either trivially exceeded by harmless blips or never
// reached during a low-traffic outage. ratio <= 0 disables it (falls back to
// WithTripThreshold). A ratio above 1 is clamped to 1 (requires 100% failures)
// so a typo can't silently produce a breaker that never trips on rate.
// minRequests below 1 is clamped to 1, since a minimum of zero would let a
// single failed request trip the breaker at a 1/1 = 100% rate, which is the
// hair trigger this option exists to avoid.
func WithFailureRatio(ratio float64, minRequests int) applyConfig {
	return func(c *config) {
		if ratio > 1 {
			ratio = 1
		}
		if minRequests < 1 {
			minRequests = 1
		}
		c.failureRatio = ratio
		c.minRequests = minRequests
	}
}

// WithTimeout sets how long the circuit remains [Open] before transitioning
// to [HalfOpen] to probe for recovery. Defaults to 1 minute. Non-positive
// values are ignored and leave the default in place, because a zero timeout
// would let the circuit leave [Open] on the very next request and defeat the
// fail-fast behaviour the open state exists to provide.
func WithTimeout(timeout time.Duration) applyConfig {
	return func(c *config) {
		if timeout <= 0 {
			return
		}
		c.timeout = timeout
	}
}

// WithClock sets a custom clock for timing operations. This is primarily
// useful for testing to control time progression. A nil clock is ignored so
// the default system clock remains in place.
func WithClock(clock clock.Clock) applyConfig {
	return func(c *config) {
		if clock == nil {
			return
		}
		c.clock = clock
	}
}

// applyConfig is a functional option for configuring a circuit breaker.
// Use the With* functions to create options.
type applyConfig func(*config)

// New creates a new circuit breaker with the given name and configuration
// options. The name is used for metrics and tracing identification. The
// circuit breaker starts in the [Closed] state, allowing all requests through.
func New[Res any](name string, applyConfigs ...applyConfig) *CB[Res] {
	cfg := &config{
		name:         name,
		maxRequests:  10,
		cyclicPeriod: 5 * time.Second,
		timeout:      time.Minute,
		isDownstreamError: func(err error) bool {
			return err != nil
		},
		tripThreshold: 5,
		failureRatio:  0,
		minRequests:   0,
		clock:         clock.New(),
	}

	for _, apply := range applyConfigs {
		apply(cfg)
	}

	cb := &CB[Res]{
		Mutex:                sync.Mutex{},
		config:               cfg,
		state:                Closed,
		generation:           0,
		resetCountersAt:      cfg.clock.Now().Add(cfg.cyclicPeriod),
		resetStateAt:         time.Time{},
		requests:             0,
		successes:            0,
		failures:             0,
		consecutiveSuccesses: 0,
		consecutiveFailures:  0,
		halfOpenRequests:     0,
	}

	return cb
}

var _ CircuitBreaker[any] = (*CB[any])(nil)

// Do executes fn if the circuit allows it. Returns [ErrTripped] immediately
// if the circuit is [Open], or [ErrTooManyRequests] if in [HalfOpen] state
// and the probe limit is exceeded. On success or failure, the result is
// recorded to update the circuit state. The zero value of Res is returned
// when the circuit rejects the request.
func (cb *CB[Res]) Do(ctx context.Context, fn func(context.Context) (Res, error)) (res Res, err error) {
	generation, err := cb.preflight(ctx)
	if err != nil {
		return res, err
	}

	// fn and isDownstreamError are both caller-supplied and may panic. Without
	// this guard the panic would unwind past the accounting below: in HalfOpen
	// the probe slot reserved by preflight would leak, wedging the breaker in
	// HalfOpen forever once maxRequests probes have panicked, and in Closed a
	// panicking dependency would never be counted as a failure and never trip
	// the circuit. The panic is recorded as a downstream failure and then
	// allowed to continue unwinding, so callers see it unchanged.
	settled := false
	defer func() {
		if !settled {
			cb.postflight(generation, true)
		}
	}()

	res, err = fn(ctx)

	// The classifier runs before the lock is taken, so user code never executes
	// inside the breaker's critical section. settled flips only once both
	// caller-supplied functions have returned, so a panic in either is still
	// accounted for exactly once.
	failed := cb.config.isDownstreamError(err)
	settled = true
	cb.postflight(generation, failed)

	return res, err
}

// preflight checks if the circuit is ready to accept a request. It returns the
// generation the request was admitted under, which postflight uses to discard
// results that no longer belong to the current state.
func (cb *CB[Res]) preflight(_ context.Context) (uint64, error) {
	cb.Lock()

	now := cb.config.clock.Now()

	if cb.state == Closed && !now.Before(cb.resetCountersAt) {
		cb.resetCounts(now)
	}
	if cb.state == Open && !now.Before(cb.resetStateAt) {
		cb.transitionTo(HalfOpen, now)
	}

	state := cb.state
	generation := cb.generation
	requests := cb.requests
	halfOpenRequests := cb.halfOpenRequests

	var err error
	switch state {
	case Open:
		err = ErrTripped
	case HalfOpen:
		if cb.halfOpenRequests >= cb.config.maxRequests {
			err = ErrTooManyRequests
		} else {
			cb.halfOpenRequests++
		}
	case Closed:
	}

	cb.Unlock()

	// Metrics and logging run outside the critical section: they happen on
	// every request through the breaker and must not extend the lock hold.
	metrics.Configured().RecordRequest(cb.config.name, string(state))
	if err != nil {
		metrics.Configured().RecordError(cb.config.name, rejectionReason(err))
	}

	logger.Debug("circuit breaker state", "state", string(state), "requests", requests, "halfOpenRequests", halfOpenRequests, "maxRequests", cb.config.maxRequests)

	return generation, err
}

// rejectionReason maps a preflight rejection to a low-cardinality metric label.
func rejectionReason(err error) string {
	if IsErrTooManyRequests(err) {
		return "too_many_requests"
	}
	return "tripped"
}

// postflight updates the circuit breaker state based on the result of the
// request. failed reports whether the result counts as a downstream failure.
func (cb *CB[Res]) postflight(generation uint64, failed bool) {
	cb.Lock()
	defer cb.Unlock()

	if generation != cb.generation {
		// The breaker changed state while this request was in flight, so the
		// result describes a regime that no longer exists. Applying it would
		// let a straggler admitted while Closed re-open a circuit that has
		// since started probing, or close one on a success that was never a
		// probe.
		return
	}

	cb.requests++
	if failed {
		cb.failures++
		cb.consecutiveFailures++
		cb.consecutiveSuccesses = 0
	} else {
		cb.successes++
		cb.consecutiveSuccesses++
		cb.consecutiveFailures = 0
	}

	now := cb.config.clock.Now()

	switch cb.state {
	case Closed:
		if cb.shouldTrip() {
			cb.transitionTo(Open, now)
		}

	case HalfOpen:
		if failed {
			cb.transitionTo(Open, now)
			break
		}
		if cb.consecutiveSuccesses >= cb.config.maxRequests {
			cb.transitionTo(Closed, now)
		}

	case Open:
	}
}

// transitionTo moves the breaker into state, starts a fresh generation so
// results from requests admitted under the previous state are discarded, and
// clears the per-cycle counters. Caller must hold the lock.
func (cb *CB[Res]) transitionTo(state State, now time.Time) {
	cb.state = state
	cb.generation++
	if state == Open {
		cb.resetStateAt = now.Add(cb.config.timeout)
	}
	cb.resetCounts(now)
}

// resetCounts clears the per-cycle counters and schedules the next cyclic
// reset. It deliberately does not bump the generation: a cyclic reset is not a
// state change, so in-flight requests still count toward the new window.
// Caller must hold the lock.
func (cb *CB[Res]) resetCounts(now time.Time) {
	cb.requests = 0
	cb.successes = 0
	cb.failures = 0
	cb.consecutiveSuccesses = 0
	cb.consecutiveFailures = 0
	cb.halfOpenRequests = 0
	cb.resetCountersAt = now.Add(cb.config.cyclicPeriod)
}

// shouldTrip reports whether the accumulated counts in the current cyclic
// period warrant opening the circuit. Rate-based when failureRatio is set,
// otherwise the legacy absolute count. Caller must hold the lock.
func (cb *CB[Res]) shouldTrip() bool {
	if cb.config.failureRatio > 0 {
		if cb.requests < cb.config.minRequests {
			return false
		}
		return float64(cb.failures)/float64(cb.requests) >= cb.config.failureRatio
	}
	return cb.failures >= cb.config.tripThreshold
}
