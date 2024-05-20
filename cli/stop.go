package main

type stopOptions struct {
	WorkloadId           string `required:"" placeholder:"<workload_id>" help:"ID of the workload to stop" group:"Stop Configuration" json:"stop_workload_id"`
	TargetNode           string `required:"" placeholder:"SDERPMON..." help:"Node to stop the workload on" group:"Stop Configuration" json:"stop_target_node"`
	ClaimsIssuerFilePath string `required:"" placeholder:"./path/to/nkey.nk" help:"Path to the claims issuer nkey file." group:"Stop Configuration" json:"stop_claims_issuer_file"`
}

func (s stopOptions) Run(cfg globals) error {
	if cfg.Check {
		return s.Table()
	}
	return nil
}

func (s stopOptions) Validate() error {
	return nil
}
