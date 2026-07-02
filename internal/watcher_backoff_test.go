package internal

import (
	"context"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
)

func TestBackoffBeforeRestartDelayGrowsAndCaps(t *testing.T) {
	tt := []struct {
		attempt int
		want    time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{6, restartBackoffMax},
		{100, restartBackoffMax}, // must not overflow the shift
	}
	for _, tc := range tt {
		be.Equal(t, tc.want, restartBackoffDelay(tc.attempt))
	}
}

func TestBackoffBeforeRestartHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &AgentWatcher{ctx: ctx}

	cancel()
	start := time.Now()
	be.False(t, w.backoffBeforeRestart(6)) // would otherwise sleep 30s
	be.True(t, time.Since(start) < time.Second)
}

func TestBackoffBeforeRestartSleeps(t *testing.T) {
	w := &AgentWatcher{ctx: context.Background()}

	start := time.Now()
	be.True(t, w.backoffBeforeRestart(1))
	// Sleep is jittered within [delay/2, delay].
	be.True(t, time.Since(start) >= restartBackoffBase/2)
}
