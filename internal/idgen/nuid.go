package idgen

import (
	"github.com/nats-io/nuid"
	"github.com/synadia-labs/nex/models"
)

type NuidGen struct {
	nuid *nuid.NUID
}

func NewNuidGen() *NuidGen {
	return &NuidGen{
		nuid: nuid.New(),
	}
}

func (n *NuidGen) Generate(_ *models.StartWorkloadRequest) string {
	return n.nuid.Next()
}
