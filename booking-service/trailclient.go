package main

import (
	"context"
	"log"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	trailpb "trailbooking/gen/trailpb"
)

// ResilienceConfig holds the settings for calls from Service A to Service B.
type ResilienceConfig struct {
	Timeout         time.Duration // max time for ONE attempt
	MaxAttempts     int           // total attempts (1 first try + retries)
	BaseBackoff     time.Duration // wait before 1st retry; doubles each retry
	BreakerFailures uint32        // consecutive failed calls before the circuit opens
	BreakerOpenFor  time.Duration // how long the circuit stays open before a trial call
}

// TrailClient wraps the generated gRPC client with three resilience patterns:
//  1. TIMEOUT          - every attempt has a deadline, so we never hang
//  2. RETRY + BACKOFF  - temporary failures are retried with growing delays
//  3. CIRCUIT BREAKER  - after repeated failures we stop calling Service B for
//                        a while and fail fast instead of piling up requests
type TrailClient struct {
	client  trailpb.TrailServiceClient
	cfg     ResilienceConfig
	breaker *gobreaker.CircuitBreaker[*trailpb.Trail]
}

func NewTrailClient(conn grpc.ClientConnInterface, cfg ResilienceConfig) *TrailClient {
	tc := &TrailClient{client: trailpb.NewTrailServiceClient(conn), cfg: cfg}

	tc.breaker = gobreaker.NewCircuitBreaker[*trailpb.Trail](gobreaker.Settings{
		Name:        "trail-service",
		MaxRequests: 1,              // trial calls allowed while half-open
		Timeout:     cfg.BreakerOpenFor, // open -> half-open after this long
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= cfg.BreakerFailures
		},
		// Business errors (NOT_FOUND, INVALID_ARGUMENT) mean Service B is
		// healthy, so they must not count as failures.
		IsSuccessful: func(err error) bool {
			return err == nil || !isRetryable(err)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("circuit breaker %q: %s -> %s", name, from, to)
		},
	})
	return tc
}

// GetTrail asks Service B for a trail, with all resilience patterns applied.
func (c *TrailClient) GetTrail(ctx context.Context, id string) (*trailpb.Trail, error) {
	return c.breaker.Execute(func() (*trailpb.Trail, error) {
		return c.getWithRetry(ctx, id)
	})
}

// getWithRetry performs up to MaxAttempts calls, each with its own timeout,
// waiting exponentially longer between attempts (200ms, 400ms, 800ms, ...).
func (c *TrailClient) getWithRetry(ctx context.Context, id string) (*trailpb.Trail, error) {
	backoff := c.cfg.BaseBackoff
	var lastErr error

	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		trail, err := c.client.GetTrail(callCtx, &trailpb.GetTrailRequest{Id: id})
		cancel()

		if err == nil {
			return trail, nil
		}
		lastErr = err

		// Do not retry errors that retrying cannot fix (e.g. NOT_FOUND).
		if !isRetryable(err) {
			return nil, err
		}
		log.Printf("trail service call failed (attempt %d/%d): %v", attempt, c.cfg.MaxAttempts, err)

		if attempt == c.cfg.MaxAttempts {
			break
		}
		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
	}
	return nil, lastErr
}

// isRetryable reports whether an error is temporary (worth retrying).
func isRetryable(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	}
	return false
}
