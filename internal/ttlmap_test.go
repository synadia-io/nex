package internal

import (
	"testing"
	"time"

	"github.com/carlmjohnson/be"
)

func waitFor(t testing.TB, timeout time.Duration, condition func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting: %s", msg)
}

func TestTTLMap(t *testing.T) {
	m := NewTTLMap(time.Second)
	m.Put("key", "value", nil)
	be.True(t, m.Exists("key"))
	v, _ := m.Get("key")
	be.Equal(t, "value", v)
	waitFor(t, 5*time.Second, func() bool {
		v, _ := m.Get("key")
		return v == ""
	}, "waiting for TTL expiry")
	v, _ = m.Get("key")
	be.Equal(t, "", v)
}

func TestTTLDelete(t *testing.T) {
	m := NewTTLMap(10 * time.Second)
	m.Put("key", "value", nil)
	be.True(t, m.Exists("key"))
	m.Delete("key")
	be.False(t, m.Exists("key"))
}
