package state

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-io/nex/models"
)

func TestNoState(t *testing.T) {
	n := &NoState{}
	be.Nonzero(t, n)

	be.Zero(t, storeErr(n.StoreWorkload("asdf", models.StartWorkloadRequest{}, 0)))
	be.Zero(t, storeErr(n.StoreWorkload("asdf", models.StartWorkloadRequest{}, 42)))
	be.Zero(t, n.RemoveWorkload("asdf", "asdf"))

	s, err := n.GetStateByAgent("asdf")
	be.NilErr(t, err)
	be.Equal(t, len(s), 0)

	s, err = n.GetStateByNamespace("asdf")
	be.NilErr(t, err)
	be.Equal(t, len(s), 0)

	// NoState stores nothing, so every record read is a clean not-found:
	// nil record, revision 0, no error -- the same signal natsKVState gives
	// for a key that is not there, which is what lets a caller feed the
	// revision straight back into StoreWorkload as "create-only".
	rec, rev, err := n.GetWorkloadRecord("asdf", "asdf")
	be.NilErr(t, err)
	be.Zero(t, rec)
	be.Equal(t, uint64(0), rev)
}
