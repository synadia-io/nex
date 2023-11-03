package nexcli

import (
	"github.com/choria-io/fisk"
)

func RunWorkload(opts *Options) func(*fisk.ParseContext) error {
	_, err := generateConnectionFromOpts(opts)
	if err != nil {
		return errorClosure(err)
	}
	//	nodeClient := controlapi.NewApiClient(nc, opts.Timeout)

	return func(ctx *fisk.ParseContext) error {
		return nil
	}
}
