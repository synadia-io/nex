package idgen

import (
	"sync"

	"github.com/nats-io/nuid"
	"github.com/synadia-io/nex/models"
)

// NuidGen is safe for concurrent use. A single generator is shared by the
// node's control endpoints (auction, deploy, remote-agent registration),
// which are dispatched on independent goroutines; *nuid.NUID's own Next() is
// not synchronized (only the package-level nuid.Next() is), so an unguarded
// shared instance is a data race and can hand out torn -- possibly duplicate
// -- ids, and two workloads sharing one id share one KV state key.
type NuidGen struct {
	mu   sync.Mutex
	nuid *nuid.NUID
}

func NewNuidGen() *NuidGen {
	return &NuidGen{
		nuid: nuid.New(),
	}
}

func (n *NuidGen) Generate(_ *models.StartWorkloadRequest) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.nuid.Next()
}
