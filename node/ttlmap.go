package node

import (
	"sync"
	"time"
)

type item struct {
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
		for _ = range time.Tick(time.Second) {
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

func (m *TTLMap) Put(k, v string) {
	m.l.Lock()
	it, ok := m.m[k]
	if !ok {
		it = &item{value: v}
		m.m[k] = it
	}
	it.createdAt = time.Now()
	m.l.Unlock()
}

func (m *TTLMap) Get(k string) (v string) {
	m.l.Lock()
	if it, ok := m.m[k]; ok {
		v = it.value
	}
	m.l.Unlock()
	return
}

func (m *TTLMap) Exists(k string) (ok bool) {
	m.l.Lock()
	_, ok = m.m[k]
	m.l.Unlock()
	return
}

func (m *TTLMap) Delete(k string) {
	m.l.Lock()
	delete(m.m, k)
	m.l.Unlock()
}
