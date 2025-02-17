package internal

import (
	"testing"
	"time"

	"github.com/carlmjohnson/be"
)

func TestTTLMap(t *testing.T) {
	m := NewTTLMap(time.Second)
	m.Put("key", "value", nil)
	be.True(t, m.Exists("key"))
	v, _ := m.Get("key")
	be.Equal(t, "value", v)
	time.Sleep(time.Second)
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
