package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRunDesktopProxyWithRetryReturnsAfterSuccess(t *testing.T) {
	calls := 0

	runDesktopProxyWithRetry(context.Background(), func() error {
		calls++
		return nil
	}, time.Millisecond, nil)

	if calls != 1 {
		t.Fatalf("expected one start call, got %d", calls)
	}
}

func TestRunDesktopProxyWithRetryRetriesUntilSuccess(t *testing.T) {
	calls := 0
	retries := 0

	runDesktopProxyWithRetry(context.Background(), func() error {
		calls++
		if calls == 1 {
			return errors.New("port in use")
		}
		return nil
	}, time.Millisecond, func(error, time.Duration) {
		retries++
	})

	if calls != 2 {
		t.Fatalf("expected two start calls, got %d", calls)
	}
	if retries != 1 {
		t.Fatalf("expected one retry callback, got %d", retries)
	}
}

func TestRunDesktopProxyWithRetryStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	retries := 0

	runDesktopProxyWithRetry(ctx, func() error {
		calls++
		cancel()
		return errors.New("port in use")
	}, time.Millisecond, func(error, time.Duration) {
		retries++
	})

	if calls != 1 {
		t.Fatalf("expected one start call before cancellation, got %d", calls)
	}
	if retries != 0 {
		t.Fatalf("expected no retry callback after cancellation, got %d", retries)
	}
}

func TestRunDesktopProxyWithRetryStopsOnServerClosed(t *testing.T) {
	calls := 0

	runDesktopProxyWithRetry(context.Background(), func() error {
		calls++
		return http.ErrServerClosed
	}, time.Millisecond, nil)

	if calls != 1 {
		t.Fatalf("expected one start call, got %d", calls)
	}
}
