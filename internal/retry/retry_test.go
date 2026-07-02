package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoSucceedsFirstAttempt(t *testing.T) {
	calls := 0
	got, err := Do(context.Background(), Policy{Attempts: 5, Base: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() (int, error) {
		calls++
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 || calls != 1 {
		t.Fatalf("got=%d calls=%d, want got=42 calls=1", got, calls)
	}
}

func TestDoSucceedsAfterFailures(t *testing.T) {
	calls := 0
	got, err := Do(context.Background(), Policy{Attempts: 5, Base: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("transient")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" || calls != 3 {
		t.Fatalf("got=%q calls=%d, want got=\"ok\" calls=3", got, calls)
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	calls := 0
	sentinel := errors.New("always fails")
	_, err := Do(context.Background(), Policy{Attempts: 3, Base: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() (int, error) {
		calls++
		return 0, sentinel
	})
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want wrapped sentinel", err)
	}
}

func TestDoZeroAttemptsCoercedToOne(t *testing.T) {
	calls := 0
	_, err := Do(context.Background(), Policy{Attempts: 0, Base: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() (int, error) {
		calls++
		return 0, errors.New("nope")
	})
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDoHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	sentinel := errors.New("transient")
	_, err := Do(ctx, Policy{Attempts: 100, Base: time.Hour, MaxDelay: time.Hour}, func() (int, error) {
		calls++
		cancel()
		return 0, sentinel
	})
	if calls != 1 {
		t.Fatalf("calls=%d, want 1 (cancel should stop retries during sleep)", calls)
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want joined context.Canceled + sentinel", err)
	}
}

func TestDoAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := Do(ctx, Policy{Attempts: 5, Base: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() (int, error) {
		calls++
		return 0, errors.New("nope")
	})
	if calls != 0 {
		t.Fatalf("calls=%d, want 0", calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
}

func TestDoBacksOffBetweenAttempts(t *testing.T) {
	start := time.Now()
	_, _ = Do(context.Background(), Policy{Attempts: 3, Base: 20 * time.Millisecond, MaxDelay: 100 * time.Millisecond}, func() (int, error) {
		return 0, errors.New("nope")
	})
	// Two sleeps: jittered [10,20]ms + [20,40]ms => at least 30ms total.
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("elapsed=%v, want >=30ms of backoff", elapsed)
	}
}

func TestDoPermanentStopsRetry(t *testing.T) {
	calls := 0
	sentinel := errors.New("not idempotent")
	_, err := Do(context.Background(), Policy{Attempts: 5, Base: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() (int, error) {
		calls++
		return 0, Permanent(sentinel)
	})
	if calls != 1 {
		t.Fatalf("calls=%d, want 1 (Permanent must stop retries)", calls)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want the original sentinel", err)
	}
	if got := err.Error(); got != sentinel.Error() {
		t.Fatalf("err=%q, want unwrapped original %q", got, sentinel.Error())
	}
}

func TestJitterBounds(t *testing.T) {
	if got := Jitter(0); got != 0 {
		t.Fatalf("Jitter(0)=%v, want 0", got)
	}
	for range 100 {
		d := Jitter(time.Second)
		if d < 500*time.Millisecond || d > time.Second {
			t.Fatalf("Jitter(1s)=%v, want within [500ms, 1s]", d)
		}
	}
}
