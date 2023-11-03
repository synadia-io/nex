package nexcli

import (
	"github.com/choria-io/fisk"
)

func RunWorkload(ctx *fisk.ParseContext) error {
	_, err := generateConnectionFromOpts()
	if err != nil {
		return err
	}
	//	nodeClient := controlapi.NewApiClient(nc, opts.Timeout)
	return nil

}
