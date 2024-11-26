package models

const (
	WorkloadRunTypeService = "service"
	WorkloadRunTypeJob     = "job"
	WorkloadRunTypeOnce    = "once"

	WorkloadStateInitializing = "initializing"
	WorkloadStateRunning      = "running"
	WorkloadStateStopped      = "stopped"
	WorkloadStateError        = "error"
	WorkloadStateWarm         = "warm"
)
