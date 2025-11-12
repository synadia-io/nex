package state

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/synadia-io/nex/models"
)

func TestNoState(t *testing.T) {
	n := &NoState{}
	be.Nonzero(t, n)

	be.Zero(t, n.StoreWorkload("asdf", models.StartWorkloadRequest{}))
	be.Zero(t, n.RemoveWorkload("asdf", "asdf"))

	s, err := n.GetStateByAgent("asdf")
	be.NilErr(t, err)
	be.Equal(t, len(s), 0)

	s, err = n.GetStateByNamespace("asdf")
	be.NilErr(t, err)
	be.Equal(t, len(s), 0)
}
