package idgen

import (
	"testing"

	"github.com/carlmjohnson/be"
)

func TestNuidGenerate(t *testing.T) {
	n := NewNuidGen()
	nuid := n.Generate(nil)
	be.Nonzero(t, nuid)
}
