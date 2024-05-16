package preflight

type PreflightCmd struct {
	ForceDepInstall bool `name:"force" default:"false" help:"Install missing dependencies without prompt" json:"preflight_force"`
}
