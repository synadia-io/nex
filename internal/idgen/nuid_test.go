package idgen

import (
	"sync"
	"testing"

	"github.com/carlmjohnson/be"
)

func TestNuidGenerate(t *testing.T) {
	n := NewNuidGen()
	nuid := n.Generate(nil)
	be.Nonzero(t, nuid)
}

// TestNuidGenerateConcurrent pins the concurrency fix: one generator is shared
// by the node's control endpoints (auction, deploy, remote registration),
// which run on independent goroutines. A bare *nuid.NUID's Next() is
// unsynchronized, so an unguarded shared instance both races (caught by -race)
// and can hand out torn -- possibly duplicate -- ids. Run this package with
// -race to exercise the guard; the uniqueness check catches a torn id even
// without the race detector.
func TestNuidGenerateConcurrent(t *testing.T) {
	n := NewNuidGen()

	const goroutines = 50
	const perGoroutine = 200

	var wg sync.WaitGroup
	results := make([][]string, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			ids := make([]string, perGoroutine)
			for i := 0; i < perGoroutine; i++ {
				ids[i] = n.Generate(nil)
			}
			results[g] = ids
		}(g)
	}
	wg.Wait()

	seen := make(map[string]struct{}, goroutines*perGoroutine)
	for _, ids := range results {
		for _, id := range ids {
			be.Nonzero(t, id)
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate id generated concurrently: %q", id)
			}
			seen[id] = struct{}{}
		}
	}
	be.Equal(t, goroutines*perGoroutine, len(seen))
}
