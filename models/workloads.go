package models

const (
	WorkloadRunTypeService  = "service"
	WorkloadRunTypeFunction = "function"
	WorkloadRunTypeJob      = "job"

	WorkloadStateInitializing = "initializing"
	WorkloadStateRunning      = "running"
	WorkloadStateStopped      = "stopped"
	WorkloadStateError        = "error"
	WorkloadStateWarm         = "warm"
)
