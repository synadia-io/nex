package internal

import (
	"sync"
	"time"

	"github.com/nats-io/nkeys"
)

type item struct {
	xkpair    nkeys.KeyPair
	value     string
	createdAt time.Time
}

type TTLMap struct {
	m map[string]*item
	l sync.Mutex
}

func NewTTLMap(lifetime time.Duration) (m *TTLMap) {
	m = &TTLMap{m: make(map[string]*item)}
	go func() {
		for range time.Tick(time.Second) {
			m.l.Lock()
			for k, v := range m.m {
				expiryTime := v.createdAt.Add(lifetime)
				if time.Now().After(expiryTime) {
					delete(m.m, k)
				}
			}
			m.l.Unlock()
		}
	}()
	return
}

func (m *TTLMap) Put(k, value string, kp nkeys.KeyPair) {
	m.l.Lock()
	it, ok := m.m[k]
	if !ok {
		it = &item{value: value, xkpair: kp}
		m.m[k] = it
	}
	it.createdAt = time.Now()
	m.l.Unlock()
}

func (m *TTLMap) Get(k string) (string, nkeys.KeyPair) {
	m.l.Lock()
	var v string
	var kp nkeys.KeyPair
	if it, ok := m.m[k]; ok {
		v = it.value
		kp = it.xkpair
	}
	m.l.Unlock()
	return v, kp
}

func (m *TTLMap) Exists(k string) bool {
	m.l.Lock()
	_, ok := m.m[k]
	m.l.Unlock()
	return ok
}

func (m *TTLMap) Delete(k string) {
	m.l.Lock()
	delete(m.m, k)
	m.l.Unlock()
}
